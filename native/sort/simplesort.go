package sort

import "github.com/hilthontt/sakura-lang/native/constraints"

// SimpleSort by skipping an unnecessary comparison of the first and last.
func Simple[T constraints.Ordered](arr []T) []T {
	for i := 1; i < len(arr); i++ {
		for j := 0; j < len(arr)-1; j++ {
			if arr[i] < arr[j] {
				// swap arr[i] and arr[j]
				arr[i], arr[j] = arr[j], arr[i]
			}
		}
	}
	return arr
}
