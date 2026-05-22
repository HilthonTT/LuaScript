package sort

import "github.com/hilthontt/sakura-lang/native/constraints"

func Bubble[T constraints.Ordered](arr []T) []T {
	swapped := true

	for swapped {
		swapped = false
		for i := 0; i < len(arr)-1; i++ {
			if arr[i+1] < arr[i] {
				arr[i+1], arr[i] = arr[i], arr[i+1]
				swapped = true
			}
		}
	}

	return arr
}
