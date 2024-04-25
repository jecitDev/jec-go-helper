package slicetools

import "sort"

func DeleteElement(slice []interface{}, index int) interface{} {
	return append(slice[:index], slice[index+1:]...)
}

func DeleteElements(slice []interface{}, indices []int) interface{} {
	// Sort indices in descending order
	sort.Sort(sort.Reverse(sort.IntSlice(indices)))

	for _, index := range indices {
		slice = append(slice[:index], slice[index+1:]...)
	}

	return slice
}
