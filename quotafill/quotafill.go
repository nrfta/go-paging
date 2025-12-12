package quotafill

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/nrfta/go-paging"
)

// Default configuration values
const (
	defaultMaxIterations      = 5
	defaultMaxRecordsExamined = 100
	defaultTimeout            = 3 * time.Second
	defaultPageSize           = 50
)

var defaultBackoffMultipliers = []int{1, 2, 3, 5, 8}

// Wrapper adapts a fetcher with quota-fill filtering to implement the Paginator interface.
type Wrapper[T any] struct {
	fetcher            paging.Fetcher[T]
	filter             paging.FilterFunc[T]
	encoder            paging.CursorEncoder[T]
	orderBy            []paging.OrderBy
	maxIterations      int
	maxRecordsExamined int
	timeout            time.Duration
	backoffMultipliers []int
}

// Option configures a quota-fill wrapper.
type Option func(*config)

// config holds wrapper configuration.
type config struct {
	maxIterations      int
	maxRecordsExamined int
	timeout            time.Duration
	backoffMultipliers []int
}

func WithMaxIterations(n int) Option {
	return func(c *config) {
		c.maxIterations = n
	}
}

func WithMaxRecordsExamined(n int) Option {
	return func(c *config) {
		c.maxRecordsExamined = n
	}
}

func WithTimeout(d time.Duration) Option {
	return func(c *config) {
		c.timeout = d
	}
}

func WithBackoffMultipliers(multipliers []int) Option {
	return func(c *config) {
		c.backoffMultipliers = multipliers
	}
}

// Safeguard identifiers returned in Metadata.SafeguardHit
const (
	safeguardTimeout       = "timeout"
	safeguardMaxRecords    = "max_records"
	safeguardMaxIterations = "max_iterations"
)

// getMultiplier returns the backoff multiplier for the given iteration.
func (w *Wrapper[T]) getMultiplier(iteration int) int {
	return w.backoffMultipliers[min(iteration, len(w.backoffMultipliers)-1)]
}

// getRequestedSize extracts the requested page size from args, defaulting to defaultPageSize.
func getRequestedSize(args *paging.PageArgs) int {
	if args != nil && args.GetFirst() != nil && *args.GetFirst() > 0 {
		return *args.GetFirst()
	}
	return defaultPageSize
}

// New creates a quota-fill paginator that adapts a fetcher with filtering.
func New[T any](
	fetcher paging.Fetcher[T],
	filter paging.FilterFunc[T],
	encoder paging.CursorEncoder[T],
	orderBy []paging.OrderBy,
	opts ...Option,
) paging.Paginator[T] {
	cfg := &config{
		maxIterations:      defaultMaxIterations,
		maxRecordsExamined: defaultMaxRecordsExamined,
		timeout:            defaultTimeout,
		backoffMultipliers: defaultBackoffMultipliers,
	}

	for _, opt := range opts {
		opt(cfg)
	}

	return &Wrapper[T]{
		fetcher:            fetcher,
		filter:             filter,
		encoder:            encoder,
		orderBy:            orderBy,
		maxIterations:      cfg.maxIterations,
		maxRecordsExamined: cfg.maxRecordsExamined,
		timeout:            cfg.timeout,
		backoffMultipliers: cfg.backoffMultipliers,
	}
}

func (w *Wrapper[T]) Paginate(ctx context.Context, args *paging.PageArgs) (*paging.Page[T], error) {
	startTime := time.Now()

	timeoutCtx, cancel := context.WithTimeout(ctx, w.timeout)
	defer cancel()

	requestedSize := getRequestedSize(args)
	targetSize := requestedSize + 1

	state := &paginationState[T]{
		currentCursor: args.GetAfter(),
	}

	for state.needsMore(targetSize) && state.iteration < w.maxIterations {
		safeguard := w.fetchIteration(timeoutCtx, args, targetSize, state)
		if state.lastError != nil {
			return nil, state.lastError
		}
		if safeguard != "" {
			state.safeguardHit = stringPtr(safeguard)
			break
		}
	}

	if state.iteration >= w.maxIterations && len(state.filteredItems) < targetSize {
		state.safeguardHit = stringPtr(safeguardMaxIterations)
	}

	return w.buildResult(state, args, requestedSize, startTime), nil
}

// paginationState tracks state across fetch iterations.
type paginationState[T any] struct {
	filteredItems []T
	examinedCount int
	iteration     int
	currentCursor *string
	safeguardHit  *string
	lastError     error
	noMoreData    bool
}

func (s *paginationState[T]) needsMore(targetSize int) bool {
	return len(s.filteredItems) < targetSize && !s.noMoreData
}

func (w *Wrapper[T]) fetchIteration(
	ctx context.Context,
	args *paging.PageArgs,
	targetSize int,
	state *paginationState[T],
) string {
	select {
	case <-ctx.Done():
		return safeguardTimeout
	default:
	}

	remaining := targetSize - len(state.filteredItems)
	batchSize := remaining * w.getMultiplier(state.iteration)
	fetchSize := batchSize + 1

	if state.examinedCount+fetchSize > w.maxRecordsExamined {
		return safeguardMaxRecords
	}

	var cursorPos *paging.CursorPosition
	if state.currentCursor != nil && w.encoder != nil {
		var err error
		cursorPos, err = w.encoder.Decode(*state.currentCursor)
		if err != nil {
			state.lastError = fmt.Errorf("decode cursor (iteration %d): %w", state.iteration+1, err)
			return ""
		}
	}

	fetchParams := paging.FetchParams{
		Limit:   fetchSize,
		Cursor:  cursorPos,
		OrderBy: w.orderBy,
	}

	items, err := w.fetcher.Fetch(ctx, fetchParams)
	if err != nil {
		if errors.Is(err, context.DeadlineExceeded) {
			return safeguardTimeout
		}
		state.lastError = fmt.Errorf("fetch batch (iteration %d): %w", state.iteration+1, err)
		return ""
	}

	hasNext := len(items) >= fetchSize

	trimmedItems := items
	if len(items) > batchSize {
		trimmedItems = items[:batchSize]
	}

	filtered, err := w.filter(ctx, trimmedItems)
	if err != nil {
		state.lastError = fmt.Errorf("apply filter (iteration %d): %w", state.iteration+1, err)
		return ""
	}

	state.filteredItems = append(state.filteredItems, filtered...)
	state.examinedCount += len(trimmedItems)
	state.iteration++

	if !hasNext {
		state.noMoreData = true
		return ""
	}

	if w.encoder != nil && len(state.filteredItems) > 0 {
		lastFilteredItem := state.filteredItems[len(state.filteredItems)-1]
		cursor, err := w.encoder.Encode(lastFilteredItem)
		if err != nil {
			state.lastError = fmt.Errorf("encode cursor from filtered item (iteration %d): %w", state.iteration, err)
			return ""
		}
		state.currentCursor = cursor
	}

	return ""
}

func (w *Wrapper[T]) buildResult(
	state *paginationState[T],
	args *paging.PageArgs,
	requestedSize int,
	startTime time.Time,
) *paging.Page[T] {
	hasNextPage := len(state.filteredItems) > requestedSize

	resultItems := state.filteredItems
	if len(resultItems) > requestedSize {
		resultItems = resultItems[:requestedSize]
	}

	pageInfo := buildPageInfo(args, hasNextPage, resultItems, w.encoder)

	return &paging.Page[T]{
		Nodes:    resultItems,
		PageInfo: &pageInfo,
		Metadata: paging.Metadata{
			Strategy:       "quotafill",
			QueryTimeMs:    time.Since(startTime).Milliseconds(),
			ItemsExamined:  state.examinedCount,
			IterationsUsed: state.iteration,
			SafeguardHit:   state.safeguardHit,
		},
	}
}

func stringPtr(s string) *string {
	return &s
}

func buildPageInfo[T any](
	args *paging.PageArgs,
	hasNextPage bool,
	items []T,
	encoder paging.CursorEncoder[T],
) paging.PageInfo {
	return paging.PageInfo{
		TotalCount: func() (*int, error) { return nil, nil },
		StartCursor: func() (*string, error) {
			if encoder == nil || len(items) == 0 {
				return nil, nil
			}
			return encoder.Encode(items[0])
		},
		EndCursor: func() (*string, error) {
			if encoder == nil || len(items) == 0 {
				return nil, nil
			}
			return encoder.Encode(items[len(items)-1])
		},
		HasNextPage: func() (bool, error) { return hasNextPage, nil },
		HasPreviousPage: func() (bool, error) {
			return args != nil && args.GetAfter() != nil, nil
		},
	}
}
