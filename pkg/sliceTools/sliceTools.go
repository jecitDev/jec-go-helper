package slicetools

import "sort"

func DeleteElement[T comparable](slice []T, index int) []T {
	return append(slice[:index], slice[index+1:]...)
}

func DeleteElements[T comparable](slice []T, indices []int) []T {
	// Sort indices in descending order
	sort.Sort(sort.Reverse(sort.IntSlice(indices)))

	for _, index := range indices {
		slice = append(slice[:index], slice[index+1:]...)
	}

	return slice
}
