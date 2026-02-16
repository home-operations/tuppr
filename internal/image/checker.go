package image

import (
	"context"
	"fmt"

	"github.com/google/go-containerregistry/pkg/name"
	"github.com/google/go-containerregistry/pkg/v1/remote"
)

type Checker struct{}

func NewChecker() *Checker {
	return &Checker{}
}

func (r *Checker) Check(ctx context.Context, imageRef string) error {
	ref, err := name.ParseReference(imageRef)
	if err != nil {
		return fmt.Errorf("failed to parse image reference %s: %w", imageRef, err)
	}
	_, err = remote.Head(ref, remote.WithContext(ctx))
	return err
}
