package slicetools

import "sort"

func DeleteElement(slice []int, index int) []int {
	return append(slice[:index], slice[index+1:]...)
}

func DeleteElements(slice []int, indices []int) []int {
	// Sort indices in descending order
	sort.Sort(sort.Reverse(sort.IntSlice(indices)))

	for _, index := range indices {
		slice = append(slice[:index], slice[index+1:]...)
	}

	return slice
}
