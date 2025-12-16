package paging

import "fmt"

const (
	// DefaultPageSize is the default number of items per page when not specified.
	DefaultPageSize = 50

	// DefaultMaxPageSize is the default maximum page size allowed.
	// This protects against resource exhaustion from unreasonably large page requests.
	DefaultMaxPageSize = 1000
)

// PageConfig holds pagination configuration options.
// Use NewPageConfig() to create a config with sensible defaults,
// then customize using the With* methods.
//
// Example:
//
//	config := paging.NewPageConfig().WithMaxSize(500)
//	limit := config.EffectiveLimit(args)
type PageConfig struct {
	// DefaultSize is the page size used when not specified in PageArgs.
	DefaultSize int

	// MaxSize is the maximum allowed page size. Requests exceeding this
	// will be capped to MaxSize (not rejected).
	MaxSize int
}

// NewPageConfig creates a PageConfig with sensible defaults:
// - DefaultSize: 50
// - MaxSize: 1000
func NewPageConfig() *PageConfig {
	return &PageConfig{
		DefaultSize: DefaultPageSize,
		MaxSize:     DefaultMaxPageSize,
	}
}

// WithDefaultSize sets the default page size and returns the config for chaining.
func (c *PageConfig) WithDefaultSize(size int) *PageConfig {
	if size > 0 {
		c.DefaultSize = size
	}
	return c
}

// WithMaxSize sets the maximum page size and returns the config for chaining.
func (c *PageConfig) WithMaxSize(size int) *PageConfig {
	if size > 0 {
		c.MaxSize = size
	}
	return c
}

// EffectiveLimit returns the page size to use, applying defaults and caps.
// - If args is nil or First is nil/zero, returns DefaultSize
// - If First exceeds MaxSize, returns MaxSize
// - Otherwise returns First
func (c *PageConfig) EffectiveLimit(args *PageArgs) int {
	if c == nil {
		c = NewPageConfig()
	}

	defaultSize := c.DefaultSize
	if defaultSize <= 0 {
		defaultSize = DefaultPageSize
	}

	maxSize := c.MaxSize
	if maxSize <= 0 {
		maxSize = DefaultMaxPageSize
	}

	if args == nil || args.First == nil || *args.First <= 0 {
		return defaultSize
	}

	if *args.First > maxSize {
		return maxSize
	}

	return *args.First
}

// Validate checks if the page size exceeds MaxSize and returns an error if so.
// Unlike EffectiveLimit which caps silently, Validate returns an error for
// explicit rejection of invalid requests.
func (c *PageConfig) Validate(args *PageArgs) error {
	if c == nil {
		c = NewPageConfig()
	}

	if args == nil || args.First == nil {
		return nil
	}

	maxSize := c.MaxSize
	if maxSize <= 0 {
		maxSize = DefaultMaxPageSize
	}

	if *args.First > maxSize {
		return &PageSizeError{
			Requested: *args.First,
			Maximum:   maxSize,
		}
	}

	return nil
}

// PageArgs represents pagination query parameters.
// It follows the Relay cursor pagination specification with First (page size),
// After (cursor), and SortBy (sort configuration) fields.
type PageArgs struct {
	First  *int    `json:"first,omitempty"`
	After  *string `json:"after,omitempty"`
	SortBy []Sort  `json:"sortBy,omitempty"`
}

// WithSortBy configures a single sort column and direction for pagination.
// It modifies the PageArgs and returns it for method chaining.
// If pa is nil, a new PageArgs is created.
//
// Example:
//
//	args := WithSortBy(nil, "created_at", true)
//	// Results in ORDER BY created_at DESC
func WithSortBy(pa *PageArgs, column string, desc bool) *PageArgs {
	if pa == nil {
		pa = &PageArgs{}
	}

	pa.SortBy = []Sort{{Column: column, Desc: desc}}
	return pa
}

// WithMultiSort configures multiple sort columns with individual directions.
// It modifies the PageArgs and returns it for method chaining.
// If pa is nil, a new PageArgs is created.
//
// Example:
//
//	args := WithMultiSort(nil,
//	    Sort{Column: "created_at", Desc: true},
//	    Sort{Column: "name", Desc: false},
//	)
//	// Results in ORDER BY created_at DESC, name ASC
func WithMultiSort(pa *PageArgs, sorts ...Sort) *PageArgs {
	if pa == nil {
		pa = &PageArgs{}
	}

	pa.SortBy = sorts
	return pa
}

// GetFirst returns the requested page size.
func (pa *PageArgs) GetFirst() *int {
	return pa.First
}

// GetAfter returns the cursor position for pagination.
func (pa *PageArgs) GetAfter() *string {
	return pa.After
}

// GetSortBy returns the list of sort specifications.
func (pa *PageArgs) GetSortBy() []Sort {
	return pa.SortBy
}

// ValidatePageSize validates that the requested page size does not exceed the maximum.
// Returns an error if First is set and exceeds maxPageSize.
//
// Deprecated: Use PageConfig.Validate() instead for clearer configuration.
// This function is kept for backwards compatibility.
//
// If maxPageSize is 0, uses DefaultMaxPageSize (1000).
//
// Example with custom limit:
//
//	func (r *resolver) Users(ctx context.Context, args *paging.PageArgs) (*UserConnection, error) {
//	    if err := paging.ValidatePageSize(args, 500); err != nil {
//	        return nil, err
//	    }
//	    // ... proceed with pagination
//	}
//
// Preferred approach using PageConfig:
//
//	config := paging.NewPageConfig().WithMaxSize(500)
//	if err := config.Validate(args); err != nil {
//	    return nil, err
//	}
func ValidatePageSize(args *PageArgs, maxPageSize int) error {
	config := NewPageConfig()
	if maxPageSize > 0 {
		config.WithMaxSize(maxPageSize)
	}
	return config.Validate(args)
}

// Validate validates the PageArgs using DefaultMaxPageSize (1000).
// This is a convenience method that uses the default PageConfig.
//
// For custom limits, use ValidateWith:
//
//	config := paging.NewPageConfig().WithMaxSize(500)
//	if err := args.ValidateWith(config); err != nil {
//	    return nil, err
//	}
func (pa *PageArgs) Validate() error {
	return NewPageConfig().Validate(pa)
}

// ValidateWith validates the PageArgs using a custom PageConfig.
// This allows specifying custom maximum page sizes per endpoint.
//
// Example:
//
//	config := paging.NewPageConfig().WithMaxSize(100)
//	if err := args.ValidateWith(config); err != nil {
//	    return nil, err // Page size too large
//	}
func (pa *PageArgs) ValidateWith(config *PageConfig) error {
	return config.Validate(pa)
}

// PageSizeError is returned when the requested page size exceeds the maximum allowed.
type PageSizeError struct {
	Requested int
	Maximum   int
}

func (e *PageSizeError) Error() string {
	return fmt.Sprintf("requested page size %d exceeds maximum allowed page size of %d",
		e.Requested, e.Maximum)
}

// PaginateOption configures page size limits for a pagination request.
// Options are passed to Paginate() to configure per-request limits.
//
// Example:
//
//	result, err := paginator.Paginate(ctx, args,
//	    paging.WithMaxSize(100),
//	    paging.WithDefaultSize(25),
//	)
type PaginateOption func(*paginateConfig)

// paginateConfig holds page size configuration for a pagination request.
type paginateConfig struct {
	maxSize     int
	defaultSize int
}

// WithMaxSize sets the maximum page size for this request.
// If the requested size exceeds this, it will be capped to maxSize.
//
// Example:
//
//	result, err := paginator.Paginate(ctx, args, paging.WithMaxSize(100))
func WithMaxSize(size int) PaginateOption {
	return func(c *paginateConfig) {
		if size > 0 {
			c.maxSize = size
		}
	}
}

// WithDefaultSize sets the default page size for this request.
// Used when args.First is nil or zero.
//
// Example:
//
//	result, err := paginator.Paginate(ctx, args, paging.WithDefaultSize(25))
func WithDefaultSize(size int) PaginateOption {
	return func(c *paginateConfig) {
		if size > 0 {
			c.defaultSize = size
		}
	}
}

// ApplyPaginateOptions applies functional options and returns a PageConfig.
// This is an internal helper used by all paginators.
func ApplyPaginateOptions(args *PageArgs, opts ...PaginateOption) *PageConfig {
	cfg := &paginateConfig{
		maxSize:     DefaultMaxPageSize,
		defaultSize: DefaultPageSize,
	}
	for _, opt := range opts {
		opt(cfg)
	}
	return &PageConfig{
		MaxSize:     cfg.maxSize,
		DefaultSize: cfg.defaultSize,
	}
}
