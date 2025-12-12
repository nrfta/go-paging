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

// Default adaptive backoff multipliers (Fibonacci-like progression)
var defaultBackoffMultipliers = []int{1, 2, 3, 5, 8}

// Wrapper wraps any paginator with quota-fill filtering.
// It implements the Paginator interface and uses the decorator pattern
// to add filtering capabilities to an existing paginator.
//
// The wrapper iteratively fetches batches from the base paginator and applies
// a filter function until the requested page size (quota) is filled or
// safeguards are triggered.
//
// Cursor Handling:
//   - For cursor-based pagination: The encoder is REQUIRED and must match the base
//     paginator's encoder. After each iteration, the cursor is generated from the
//     LAST FILTERED ITEM, ensuring cursor alignment when filtering removes items.
//   - For offset-based pagination: The encoder can be nil. Cursors are managed by
//     the base paginator (typically offset values).
//
// Type parameter T is the item type being paginated and filtered.
type Wrapper[T any] struct {
	base               paging.Paginator[T]
	filter             paging.FilterFunc[T]
	encoder            paging.CursorEncoder[T] // Required for cursor pagination, nil for offset
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

// WithMaxIterations sets the maximum number of fetch iterations.
// Default: 5
//
// This safeguard prevents infinite loops when the filter pass rate is very low.
// If the maximum is reached, partial results are returned with a safeguard warning.
func WithMaxIterations(n int) Option {
	return func(c *config) {
		c.maxIterations = n
	}
}

// WithMaxRecordsExamined sets the maximum number of records to examine.
// Default: 100
//
// This safeguard prevents excessive database load when filtering is very selective.
// If the maximum is reached, partial results are returned with a safeguard warning.
func WithMaxRecordsExamined(n int) Option {
	return func(c *config) {
		c.maxRecordsExamined = n
	}
}

// WithTimeout sets the maximum time allowed for pagination.
// Default: 3 seconds
//
// This safeguard prevents long-running queries that could impact system performance.
// If the timeout is reached, partial results are returned with a safeguard warning.
func WithTimeout(d time.Duration) Option {
	return func(c *config) {
		c.timeout = d
	}
}

// WithBackoffMultipliers sets the adaptive backoff multipliers.
// Default: [1, 2, 3, 5, 8] (Fibonacci-like progression)
//
// These multipliers adjust the fetch size based on the iteration number
// to optimize for different filter pass rates:
//   - Iteration 1: Fetch exactly what's needed (1x)
//   - Iteration 2: Filter rate < 100%, overscan (2x)
//   - Iteration 3+: Progressively larger overscan (3x, 5x, 8x)
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

// copySortConfig copies sort configuration from source to target PageArgs.
func copySortConfig(target, source *paging.PageArgs) *paging.PageArgs {
	if source != nil && len(source.SortByCols()) > 0 {
		return paging.WithSortBy(target, source.IsDesc(), source.SortByCols()...)
	}
	return target
}

// getRequestedSize extracts the requested page size from args, defaulting to defaultPageSize.
func getRequestedSize(args *paging.PageArgs) int {
	if args != nil && args.GetFirst() != nil && *args.GetFirst() > 0 {
		return *args.GetFirst()
	}
	return defaultPageSize
}

// Wrap wraps any paginator with quota-fill filtering.
// The wrapped paginator handles the actual pagination strategy (cursor/offset/etc).
// Quota-fill handles the filtering and iterative fetching.
//
// Type parameter T is the item type being paginated and filtered.
//
// Parameters:
//   - base: The base paginator to wrap (cursor or offset)
//   - filter: Filter function to apply to each batch
//   - encoder: Cursor encoder for generating cursors (required for cursor pagination, pass nil for offset)
//   - opts: Optional configuration (WithMaxIterations, WithMaxRecordsExamined, etc.)
//
// Example usage with cursor pagination and authorization:
//
//	encoder := cursor.NewCompositeCursorEncoder(func(o *models.Organization) map[string]any {
//	    return map[string]any{"created_at": o.CreatedAt, "id": o.ID}
//	})
//	basePaginator := cursor.New(pageArgs, encoder, items)
//	authFilter := func(ctx context.Context, orgs []*models.Organization) ([]*models.Organization, error) {
//	    return authzClient.FilterAuthorized(ctx, userID, orgs)
//	}
//	paginator := quotafill.Wrap(basePaginator, authFilter, encoder,
//	    quotafill.WithMaxIterations(5),
//	    quotafill.WithMaxRecordsExamined(100),
//	)
//
// Example usage with offset pagination and soft-deletes:
//
//	basePaginator := offset.New(pageArgs, items)
//	softDeleteFilter := func(ctx context.Context, posts []*models.Post) ([]*models.Post, error) {
//	    filtered := []*models.Post{}
//	    for _, post := range posts {
//	        if !post.DeletedAt.Valid {
//	            filtered = append(filtered, post)
//	        }
//	    }
//	    return filtered, nil
//	}
//	paginator := quotafill.Wrap(basePaginator, softDeleteFilter, nil)  // Pass nil encoder for offset
func Wrap[T any](
	base paging.Paginator[T],
	filter paging.FilterFunc[T],
	encoder paging.CursorEncoder[T],
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
		base:               base,
		filter:             filter,
		encoder:            encoder,
		maxIterations:      cfg.maxIterations,
		maxRecordsExamined: cfg.maxRecordsExamined,
		timeout:            cfg.timeout,
		backoffMultipliers: cfg.backoffMultipliers,
	}
}

// Paginate implements the Paginator interface with quota-fill logic.
//
// Algorithm:
//  1. Initialize: filteredItems = [], examinedCount = 0, iteration = 0
//  2. Loop while len(filteredItems) < requestedPageSize + 1 (N+1 for hasNextPage):
//     a. Check safeguards: maxIterations, maxRecordsExamined, timeout
//     b. Calculate fetchSize = (remaining quota) * backoffMultiplier[iteration]
//     c. Fetch batch from base paginator
//     d. Apply filter function to batch
//     e. Append filtered items to results
//     f. Update counters, advance cursor
//     g. Break if no more data or safeguard triggered
//  3. Trim to requestedPageSize if we got N+1
//  4. Return result with metadata (examined count, iterations, safeguards)
func (w *Wrapper[T]) Paginate(ctx context.Context, args *paging.PageArgs) (*paging.Page[T], error) {
	startTime := time.Now()

	timeoutCtx, cancel := context.WithTimeout(ctx, w.timeout)
	defer cancel()

	requestedSize := getRequestedSize(args)
	targetSize := requestedSize + 1 // N+1 pattern for hasNextPage detection

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

	// Check if max iterations safeguard was hit
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
	noMoreData    bool // true when base paginator has no more pages
}

// needsMore returns true if more items are needed to fill the quota and data is available.
func (s *paginationState[T]) needsMore(targetSize int) bool {
	return len(s.filteredItems) < targetSize && !s.noMoreData
}

// fetchIteration performs a single fetch-filter cycle. Returns safeguard name if triggered, empty string otherwise.
func (w *Wrapper[T]) fetchIteration(
	ctx context.Context,
	args *paging.PageArgs,
	targetSize int,
	state *paginationState[T],
) string {
	// Check timeout
	select {
	case <-ctx.Done():
		return safeguardTimeout
	default:
	}

	// Calculate fetch size with adaptive backoff
	remaining := targetSize - len(state.filteredItems)
	fetchSize := remaining * w.getMultiplier(state.iteration)

	// Check max records safeguard before fetching
	if state.examinedCount+fetchSize > w.maxRecordsExamined {
		return safeguardMaxRecords
	}

	// Build fetch args
	fetchArgs := copySortConfig(&paging.PageArgs{
		First: &fetchSize,
		After: state.currentCursor,
	}, args)

	// Fetch batch
	page, err := w.base.Paginate(ctx, fetchArgs)
	if err != nil {
		if errors.Is(err, context.DeadlineExceeded) {
			return safeguardTimeout
		}
		state.lastError = fmt.Errorf("fetch batch (iteration %d): %w", state.iteration+1, err)
		return ""
	}

	// Apply filter
	filtered, err := w.filter(ctx, page.Nodes)
	if err != nil {
		state.lastError = fmt.Errorf("apply filter (iteration %d): %w", state.iteration+1, err)
		return ""
	}

	// Update state
	state.filteredItems = append(state.filteredItems, filtered...)
	state.examinedCount += len(page.Nodes)
	state.iteration++

	// Check for more data and update cursor
	hasNext, err := page.PageInfo.HasNextPage()
	if err != nil {
		state.lastError = fmt.Errorf("check hasNextPage (iteration %d): %w", state.iteration, err)
		return ""
	}

	if !hasNext {
		state.noMoreData = true
		return ""
	}

	// For cursor-based pagination with encoder, generate cursor from last filtered item
	// This ensures cursor alignment when filtering removes items
	if w.encoder != nil && len(state.filteredItems) > 0 {
		lastFilteredItem := state.filteredItems[len(state.filteredItems)-1]
		cursor, err := w.encoder.Encode(lastFilteredItem)
		if err != nil {
			state.lastError = fmt.Errorf("encode cursor from filtered item (iteration %d): %w", state.iteration, err)
			return ""
		}
		state.currentCursor = cursor
		return ""
	}

	// For offset-based pagination or when no encoder provided, use base paginator's cursor
	endCursor, err := page.PageInfo.EndCursor()
	if err != nil {
		state.lastError = fmt.Errorf("get endCursor (iteration %d): %w", state.iteration, err)
		return ""
	}

	state.currentCursor = endCursor
	return ""
}

// buildResult constructs the final Page from pagination state.
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

// stringPtr returns a pointer to the given string.
func stringPtr(s string) *string {
	return &s
}

// buildPageInfo constructs PageInfo for quota-fill results.
// If encoder is provided (cursor pagination), it generates cursors for first/last items.
// If encoder is nil (offset pagination), cursors are nil.
func buildPageInfo[T any](
	args *paging.PageArgs,
	hasNextPage bool,
	items []T,
	encoder paging.CursorEncoder[T],
) paging.PageInfo {
	return paging.PageInfo{
		TotalCount: func() (*int, error) { return nil, nil },

		// StartCursor: Encode first item (or nil if empty/no encoder)
		StartCursor: func() (*string, error) {
			if encoder == nil || len(items) == 0 {
				return nil, nil
			}
			return encoder.Encode(items[0])
		},

		// EndCursor: Encode last item (or nil if empty/no encoder)
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
