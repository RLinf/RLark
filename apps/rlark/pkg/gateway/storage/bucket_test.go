package storage

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestValidateBucketCredentials(t *testing.T) {
	cases := []struct {
		name       string
		statusCode int
		wantErr    bool
	}{
		{name: "accepts accessible bucket", statusCode: http.StatusOK},
		{name: "returns access denied", statusCode: http.StatusForbidden, wantErr: true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			var method, path string
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				method = r.Method
				path = r.URL.Path
				w.WriteHeader(tc.statusCode)
			}))
			defer server.Close()

			client, err := NewClient(&Config{
				AccessKeyId: "access", SecretAccessKey: "secret", Bucket: "bucket",
				Endpoint: server.URL, Region: "us-east-1", UsePathStyle: true, Provider: "Minio",
			})
			require.NoError(t, err)

			err = client.ValidateBucketCredentials(context.Background())
			if tc.wantErr {
				require.Error(t, err)
			} else {
				require.NoError(t, err)
			}
			assert.Equal(t, http.MethodHead, method)
			assert.Equal(t, "/bucket", path)
		})
	}
}
