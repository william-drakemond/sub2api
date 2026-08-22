//go:build !darwin

package liveattestation

import "context"

type unsupportedProvider struct{}

func (unsupportedProvider) Check(context.Context) error {
	return ErrUnsupportedPlatform
}

func (unsupportedProvider) Generate(context.Context) (string, error) {
	return "", ErrUnsupportedPlatform
}
