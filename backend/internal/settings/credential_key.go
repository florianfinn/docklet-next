// Source: docklet-next/docs/design/images-updates.md
// Revision: 361c54cb8d087b7d531ff8bb4dd1acae82f5f534
// Retrieved: 2026-10-24
// License: Apache-2.0
// Deviations: Original PR1 image and registry safety implementation for issue #229.

package settings

import (
	"github.com/florianfinn/docklet-next/backend/internal/config"
	imageDTO "github.com/florianfinn/docklet-next/types/image"
	"go.getarcane.app/sys/crypto"
	"reflect"
	"strings"
)

// ValidateCredentialEncryption verifies the effective encryptor, including agent
// generated keys; checking an environment variable alone would miss that key.
func ValidateCredentialEncryption() error {
	encryptor, err := crypto.Default()
	if err != nil {
		return imageDTO.Failure("registry.credential-unavailable", 503)
	}
	probe, err := encryptor.Encrypt("docklet-credential-key-validation")
	if err != nil {
		return imageDTO.Failure("registry.credential-unavailable", 503)
	}
	field, _ := reflect.TypeFor[config.Config]().FieldByName("EncryptionKey")
	development := field.Tag.Get("default")
	fallback, _, _ := strings.Cut(development, "-32-")
	for _, value := range []string{development, fallback} {
		key, e := crypto.ParseKey(value)
		if e != nil {
			return imageDTO.Failure("registry.credential-unavailable", 503)
		}
		weak, e := crypto.New(key)
		if e == nil {
			if _, e = weak.Decrypt(probe); e == nil {
				return imageDTO.Failure("registry.credential-unavailable", 503)
			}
		}
	}
	return nil
}
