package helpers

import (
	"errors"
	"testing"
)

func TestIsAutoConnectSessionDeleted(t *testing.T) {
	cases := []struct {
		name string
		err  error
		want bool
	}{
		{
			name: "session deleted reconnect",
			err:  errors.New("device abc is not logged in (session deleted)"),
			want: true,
		},
		{
			name: "real reconnect failure",
			err:  errors.New("connect failed: websocket timeout"),
			want: false,
		},
		{
			name: "nil",
			err:  nil,
			want: false,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := IsAutoConnectSessionDeleted(tc.err); got != tc.want {
				t.Fatalf("IsAutoConnectSessionDeleted() = %v, want %v", got, tc.want)
			}
		})
	}
}
