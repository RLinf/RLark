package rlarkctl

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"

	"github.com/spf13/cobra"
)

// ProxyCurlCommand proxies the curlCommand.
func ProxyCurlCommand() *cobra.Command {
	method := "GET"
	var data string

	cmd := &cobra.Command{
		Use:   "proxy-curl [URL]",
		Short: "Make a HTTP request through the server proxy endpoint.",
		RunE: func(cmd *cobra.Command, args []string) error {
			if len(args) == 0 {
				return cmd.Help()
			}
			u, err := url.Parse(args[0])
			if err != nil {
				return fmt.Errorf("invalid URL: %w", err)
			}
			client, err := NewClient(cmd.Context())
			if err != nil {
				return err
			}
			target := client.BuildURLWithQuery(u.Query(), "api", "proxy", u.Host, u.Path)
			var resp *http.Response
			if data != "" {
				resp, err = client.DoRequestWithObject(cmd.Context(), method, target, json.RawMessage(data))
			} else {
				resp, err = client.DoRequest(cmd.Context(), method, target, nil)
			}
			if err != nil {
				return err
			}
			defer func() { _ = resp.Body.Close() }()

			_, _ = fmt.Fprintf(cmd.ErrOrStderr(), "Status: %s\n", resp.Status)
			_, _ = fmt.Fprintf(cmd.ErrOrStderr(), "Headers:\n")
			for k, v := range resp.Header {
				_, _ = fmt.Fprintf(cmd.ErrOrStderr(), "  %s: %s\n", k, v)
			}
			_, _ = fmt.Fprintln(cmd.ErrOrStderr(), "Body:")
			_, err = io.Copy(cmd.OutOrStdout(), resp.Body)
			return err
		},
	}
	return cmd
}
