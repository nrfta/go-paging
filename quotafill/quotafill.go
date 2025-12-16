package quotafill

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/nrfta/paging-go/v2"
	"github.com/nrfta/paging-go/v2/cursor"
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
	schema             *cursor.Schema[T]
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

// New creates a quota-fill paginator that adapts a fetcher with filtering.
// The schema parameter provides both the cursor encoder and sort ordering,
// ensuring they are always synchronized.
//
// Page size limits are configured per-request via Paginate() options:
//
//	paginator := quotafill.New(fetcher, filter, schema,
//	    quotafill.WithMaxIterations(10),
//	)
//	result, _ := paginator.Paginate(ctx, args, paging.WithMaxSize(100))
func New[T any](
	fetcher paging.Fetcher[T],
	filter paging.FilterFunc[T],
	schema *cursor.Schema[T],
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
		schema:             schema,
		maxIterations:      cfg.maxIterations,
		maxRecordsExamined: cfg.maxRecordsExamined,
		timeout:            cfg.timeout,
		backoffMultipliers: cfg.backoffMultipliers,
	}
}

func (w *Wrapper[T]) Paginate(ctx context.Context, args *paging.PageArgs, opts ...paging.PaginateOption) (*paging.Page[T], error) {
	startTime := time.Now()

	timeoutCtx, cancel := context.WithTimeout(ctx, w.timeout)
	defer cancel()

	// Apply per-request page size config
	pageConfig := paging.ApplyPaginateOptions(args, opts...)
	requestedSize := pageConfig.EffectiveLimit(args)
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

	// Cap fetch size to remaining examination budget
	maxAllowed := w.maxRecordsExamined - state.examinedCount
	if fetchSize > maxAllowed {
		if maxAllowed < 2 {
			// Need at least 2 records for N+1 pattern (1 to return, 1 for hasNext)
			// If budget doesn't allow this, trigger safeguard
			return safeguardMaxRecords
		}
		// Cap the fetch to remaining budget while maintaining N+1 pattern
		fetchSize = maxAllowed
		batchSize = fetchSize - 1
	}

	// Get encoder from schema for the current args
	var cursorPos *paging.CursorPosition
	if state.currentCursor != nil && w.schema != nil {
		encoder, err := w.schema.EncoderFor(argsForEncoder(args))
		if err != nil {
			state.lastError = fmt.Errorf("get encoder (iteration %d): %w", state.iteration+1, err)
			return ""
		}

		cursorPos, err = encoder.Decode(*state.currentCursor)
		if err != nil {
			state.lastError = fmt.Errorf("decode cursor (iteration %d): %w", state.iteration+1, err)
			return ""
		}
	}

	// Get orderBy from schema
	var orderBy []paging.Sort
	if w.schema != nil {
		orderBy = w.schema.BuildOrderBy(getSortBy(args))
	}

	fetchParams := paging.FetchParams{
		Limit:   fetchSize,
		Cursor:  cursorPos,
		OrderBy: orderBy,
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

	// Encode cursor from last EXAMINED item (not last filtered item)
	// This ensures we continue from where we left off in the database scan
	if w.schema != nil && len(trimmedItems) > 0 {
		encoder, err := w.schema.EncoderFor(argsForEncoder(args))
		if err != nil {
			state.lastError = fmt.Errorf("get encoder for cursor (iteration %d): %w", state.iteration, err)
			return ""
		}

		lastExaminedItem := trimmedItems[len(trimmedItems)-1]
		cursor, err := encoder.Encode(lastExaminedItem)
		if err != nil {
			state.lastError = fmt.Errorf("encode cursor from examined item (iteration %d): %w", state.iteration, err)
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

	pageInfo := buildPageInfo(args, hasNextPage, resultItems, w.schema)

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

// argsForEncoder extracts just the SortBy from args for encoder creation.
func argsForEncoder(args *paging.PageArgs) *paging.PageArgs {
	if args == nil || args.GetSortBy() == nil {
		return &paging.PageArgs{}
	}
	return &paging.PageArgs{SortBy: args.GetSortBy()}
}

// getSortBy safely extracts SortBy from args.
func getSortBy(args *paging.PageArgs) []paging.Sort {
	if args == nil || args.GetSortBy() == nil {
		return nil
	}
	return args.GetSortBy()
}

func buildPageInfo[T any](
	args *paging.PageArgs,
	hasNextPage bool,
	items []T,
	schema *cursor.Schema[T],
) paging.PageInfo {
	return paging.PageInfo{
		TotalCount: func() (*int, error) { return nil, nil },
		StartCursor: func() (*string, error) {
			if schema == nil || len(items) == 0 {
				return nil, nil
			}
			encoder, err := schema.EncoderFor(argsForEncoder(args))
			if err != nil {
				return nil, err
			}
			return encoder.Encode(items[0])
		},
		EndCursor: func() (*string, error) {
			if schema == nil || len(items) == 0 {
				return nil, nil
			}
			encoder, err := schema.EncoderFor(argsForEncoder(args))
			if err != nil {
				return nil, err
			}
			return encoder.Encode(items[len(items)-1])
		},
		HasNextPage: func() (bool, error) { return hasNextPage, nil },
		HasPreviousPage: func() (bool, error) {
			return args != nil && args.GetAfter() != nil, nil
		},
	}
}
