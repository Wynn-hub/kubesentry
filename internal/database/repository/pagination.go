package repository

import (
	"fmt"

	"gorm.io/gorm"
)

// PageOptions contains already-validated internal pagination values. Each
// repository still owns a sort whitelist so callers cannot turn SortBy into
// an arbitrary SQL fragment.
type PageOptions struct {
	Limit  int
	Offset int
	SortBy string
	Desc   bool
}

func queryPage[T any](base *gorm.DB, options PageOptions, sortFields map[string]string, stableOrder string) ([]T, int64, error) {
	if options.Limit < 1 || options.Limit > 100 {
		return nil, 0, fmt.Errorf("invalid page limit %d", options.Limit)
	}
	if options.Offset < 0 {
		return nil, 0, fmt.Errorf("invalid page offset %d", options.Offset)
	}
	sortExpression, ok := sortFields[options.SortBy]
	if !ok {
		return nil, 0, fmt.Errorf("invalid sort field %q", options.SortBy)
	}

	var total int64
	if err := base.Count(&total).Error; err != nil {
		return nil, 0, err
	}

	direction := "ASC"
	if options.Desc {
		direction = "DESC"
	}
	query := base.Order(sortExpression + " " + direction)
	if stableOrder != "" {
		query = query.Order(stableOrder)
	}

	items := make([]T, 0)
	if err := query.Limit(options.Limit).Offset(options.Offset).Find(&items).Error; err != nil {
		return nil, 0, err
	}
	return items, total, nil
}
