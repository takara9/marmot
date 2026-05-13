package util

// DerefStrPtr returns the string value if ptr is not nil, otherwise "".
func DerefStrPtr(ptr *string) string {
	if ptr == nil {
		return ""
	}
	return *ptr
}
