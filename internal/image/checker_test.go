package image

import (
	"context"
	"encoding/base64"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/google/go-containerregistry/pkg/authn"
	"github.com/google/go-containerregistry/pkg/name"
	"github.com/google/go-containerregistry/pkg/registry"
	"github.com/google/go-containerregistry/pkg/v1/empty"
	"github.com/google/go-containerregistry/pkg/v1/remote"
)

const (
	testUser     = "user"
	testPassword = "pass"
)

func newAuthRegistry(t *testing.T) string {
	t.Helper()
	reg := registry.New()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		user, pass, ok := r.BasicAuth()
		if !ok || user != testUser || pass != testPassword {
			w.Header().Set("WWW-Authenticate", `Basic realm="test"`)
			w.WriteHeader(http.StatusUnauthorized)
			return
		}
		reg.ServeHTTP(w, r)
	}))
	t.Cleanup(srv.Close)
	return strings.TrimPrefix(srv.URL, "http://")
}

func TestCheck(t *testing.T) {
	host := newAuthRegistry(t)
	imageRef := host + "/talos/installer:v1.0.0"

	ref, err := name.ParseReference(imageRef)
	if err != nil {
		t.Fatalf("parse reference: %v", err)
	}
	if err := remote.Write(ref, empty.Image, remote.WithAuth(&authn.Basic{Username: testUser, Password: testPassword})); err != nil {
		t.Fatalf("push image: %v", err)
	}

	tests := []struct {
		name         string
		dockerConfig string
		imageRef     string
		wantErr      bool
	}{
		{
			name:     "no docker config is anonymous and rejected",
			imageRef: imageRef,
			wantErr:  true,
		},
		{
			name:         "credentials from docker config are used",
			dockerConfig: fmt.Sprintf(`{"auths":{%q:{"auth":%q}}}`, host, base64.StdEncoding.EncodeToString([]byte(testUser+":"+testPassword))),
			imageRef:     imageRef,
		},
		{
			name:         "wrong credentials are rejected",
			dockerConfig: fmt.Sprintf(`{"auths":{%q:{"auth":%q}}}`, host, base64.StdEncoding.EncodeToString([]byte(testUser+":wrong"))),
			imageRef:     imageRef,
			wantErr:      true,
		},
		{
			name:         "missing tag is reported",
			dockerConfig: fmt.Sprintf(`{"auths":{%q:{"auth":%q}}}`, host, base64.StdEncoding.EncodeToString([]byte(testUser+":"+testPassword))),
			imageRef:     host + "/talos/installer:v9.9.9",
			wantErr:      true,
		},
		{
			name:     "invalid reference",
			imageRef: "not a reference",
			wantErr:  true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			dir := t.TempDir()
			t.Setenv("HOME", dir)
			t.Setenv("DOCKER_CONFIG", dir)
			if tt.dockerConfig != "" {
				if err := os.WriteFile(filepath.Join(dir, "config.json"), []byte(tt.dockerConfig), 0o600); err != nil {
					t.Fatalf("write docker config: %v", err)
				}
			}

			err := NewChecker().Check(context.Background(), tt.imageRef)
			if (err != nil) != tt.wantErr {
				t.Fatalf("Check() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}
