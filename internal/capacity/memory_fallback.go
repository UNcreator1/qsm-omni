//go:build !darwin

package capacity

func detectMemoryBytes() int64 {
	return 0
}
