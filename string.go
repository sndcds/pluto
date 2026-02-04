package pluto

import "fmt"

func stringPtrToStr(s *string) string {
	if s == nil {
		return "<nil>"
	}
	return *s
}

func floatPtrToStr(f *float64) string {
	if f == nil {
		return "<nil>"
	}
	return fmt.Sprintf("%f", *f)
}

func intPtrToStr(i *int) string {
	if i == nil {
		return "<nil>"
	}
	return fmt.Sprintf("%d", *i)
}
