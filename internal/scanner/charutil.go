package scanner

func IsAlpha[T byte | rune](b T) bool {
	return b >= 'a' && b <= 'z' || b >= 'A' && b <= 'Z' || b == '_'
}

func IsDigit[T byte | rune](b T) bool {
	return b >= '0' && b <= '9'
}

func IsAlnum[T byte | rune](b T) bool {
	return IsAlpha(b) || IsDigit(b)
}

func IsCtrl[T byte | rune](b T) bool {
	return b < 32
}
