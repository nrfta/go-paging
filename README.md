# go-paging ![](https://github.com/nrfta/go-paging/workflows/CI/badge.svg)

Go pagination for [SQLBoiler](https://github.com/aarondl/sqlboiler) and [gqlgen](https://github.com/99designs/gqlgen/) (GraphQL).

## Install

```sh
go get -u "github.com/nrfta/go-paging"
```

## Usage

1. Add [this GraphQL schema](./schema.graphql) to your project.

2. Add models to `gqlgen.yml`:

```yaml
models:
  PageArgs:
    model: github.com/nrfta/go-paging.PageArgs

  PageInfo:
    model: github.com/nrfta/go-paging.PageInfo

```

3. Add PageInfo Resolver for gqlgen

```go
package resolvers

import (
	"github.com/nrfta/go-paging"
)

func (r *RootResolver) PageInfo() PageInfoResolver {
	return paging.NewPageInfoResolver()
}
```

4. Full Example

This assumes you have the following GraphQL schema:

```graphql
type Post {
  id: ID!
  name: String
}

type PostEdge {
  cursor: String
  node: Post!
}

type PostConnection {
  edges: [PostEdge!]!
  pageInfo: PageInfo!
}

type Query {
  posts(page: PageArgs): PostConnection!
}
```

> Note that PageArgs and PageInfo is defined in [schema.graphql](./schema.graphql) and your should copy it to your project.

Here is what the resolver function would look like:

```go
package resolvers

import (
  "context"

	"github.com/nrfta/go-paging"
	"github.com/aarondl/sqlboiler/v4/queries/qm"

	"github.com/my-user/my-app/models"
)

func (r *queryResolver) Posts(ctx context.Context, page *paging.PageArgs) (*PostConnection, error) {
	var mods []qm.QueryMod

	totalCount, err := models.Posts().Count(ctx, DB)
	if err != nil {
		return &PostConnection{
			PageInfo: paging.NewEmptyPageInfo(),
		}, err
	}

	paginator := paging.NewOffsetPaginator(page, totalCount)
	mods = append(mods, paginator.QueryMods()...)

	records, err := models.Posts(mods...).All(ctx, DB)
	if err != nil {
		return &PostConnection{
			PageInfo: paging.NewEmptyPageInfo(),
		}, err
	}

	result := &PostConnection{
		PageInfo: &paginator.PageInfo,
	}

	for i, row := range records {
		result.Edges = append(result.Edges, &PostEdge{
			Cursor: paging.EncodeOffsetCursor(paginator.Offset + i + 1),
			Node:   row,
		})
	}
	return result, nil
}
```

## License

This project is licensed under the [MIT License](LICENSE.md).
