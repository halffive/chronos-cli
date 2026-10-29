package app

import (
	"iter"

	chronos "chronos.sh/go"
)

// CollectAll walks every page into one. Its paging describes the whole walk,
// with pages 0 for an empty list, as the API reports it.
func CollectAll[T any](items iter.Seq2[T, error]) (*chronos.Page[T], error) {
	data := []T{}
	for item, err := range items {
		if err != nil {
			return nil, err
		}
		data = append(data, item)
	}
	pages := 0
	if len(data) > 0 {
		pages = 1
	}
	return &chronos.Page[T]{
		Data:   data,
		Paging: chronos.Paging{Page: 1, Pages: pages, Size: len(data), Total: len(data)},
	}, nil
}
