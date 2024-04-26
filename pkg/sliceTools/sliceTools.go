package slicetools

import "sort"

func DeleteElement(slice interface{}, index int) interface{} {
	sliceSlice := slice.([]interface{})
	return append(sliceSlice[:index], sliceSlice[index+1:]...)
}

func DeleteElements(slice interface{}, indices []int) interface{} {
	// Sort indices in descending order
	sort.Sort(sort.Reverse(sort.IntSlice(indices)))

	sliceSlice := slice.([]interface{})

	for _, index := range indices {
		sliceSlice = append(sliceSlice[:index], sliceSlice[index+1:]...)
	}

	return sliceSlice
}
