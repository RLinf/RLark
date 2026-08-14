package rlarkctl

import (
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"slices"

	"github.com/spf13/cobra"

	"github.com/rlinf/rlark/apps/rlark/pkg/server"
)

// SignCommand signs the command.
func SignCommand() *cobra.Command {
	req := server.SignRequest{
		Role:     "agent",
		ClientID: "example-client-id",
	}
	var output string

	cmd := &cobra.Command{
		Use:   "sign",
		Short: "Sign a certificate.",
		RunE: func(cmd *cobra.Command, args []string) error {
			client, err := NewClient(cmd.Context())
			if err != nil {
				return err
			}

			target := client.BuildURL("api", "sign")
			resp, err := client.DoRequestWithObject(cmd.Context(), http.MethodPost, target, req)
			if err != nil {
				return err
			}
			defer func() { _ = resp.Body.Close() }()

			if resp.StatusCode != 200 {
				return fmt.Errorf("sign certificate failed with status: %s", resp.Status)
			}

			var respData server.SignResponse
			if err := json.NewDecoder(resp.Body).Decode(&respData); err != nil {
				return fmt.Errorf("failed to decode response: %w", err)
			}

			_, _ = fmt.Fprintln(cmd.ErrOrStderr(), "Certificate signed successfully.")
			_, _ = fmt.Fprintf(cmd.ErrOrStderr(), "Type: %s\n", respData.CertType)
			_, _ = fmt.Fprintf(cmd.ErrOrStderr(), "Serial Number: %s\n", respData.SerialNumber)
			_, _ = fmt.Fprintf(cmd.ErrOrStderr(), "Subject Key ID: %s\n", respData.SubjectKeyID)

			if output != "" {
				certPath := filepath.Join(output, "cert.pem")
				keyPath := filepath.Join(output, "key.pem")
				if err := os.WriteFile(certPath, []byte(respData.CertPEM), 0600); err != nil {
					return fmt.Errorf("failed to write certificate: %w", err)
				}
				if err := os.WriteFile(keyPath, []byte(respData.KeyPEM), 0600); err != nil {
					return fmt.Errorf("failed to write private key: %w", err)
				}
			} else {
				fmt.Println("Certificate:")
				fmt.Println(respData.CertPEM)
				fmt.Println("Private Key:")
				fmt.Println(respData.KeyPEM)
			}

			return nil
		},
	}

	cmd.Flags().StringVarP(&req.Role, "role", "r", req.Role, "Role for the certificate (e.g. admin, peer, agent)")
	cmd.Flags().StringVarP(&req.ClientID, "client-id", "c", req.ClientID, "Client ID for agent role (optional)")
	cmd.Flags().StringVarP(&output, "output", "o", "", "Output directory for the signed certificate and key (optional)")

	return cmd
}

// RevokeCommand revokes the command.
func RevokeCommand() *cobra.Command {
	var certType, serialNumber, subjectKeyID, reason string

	cmd := &cobra.Command{
		Use:   "revoke",
		Short: "Revoke a certificate.",
		RunE: func(cmd *cobra.Command, args []string) error {
			if certType == "" || serialNumber == "" || subjectKeyID == "" {
				return fmt.Errorf("cert-type, serial-number, and subject-key-id are required")
			}
			if !slices.Contains([]string{"x509", "ssh"}, certType) {
				return fmt.Errorf("invalid cert-type: %s", certType)
			}

			client, err := NewClient(cmd.Context())
			if err != nil {
				return err
			}

			req := server.RevokeCertRequest{
				CertType:     certType,
				SerialNumber: serialNumber,
				SubjectKeyID: subjectKeyID,
				Reason:       reason,
			}

			target := client.BuildURL("api", "revoke")
			resp, err := client.DoRequestWithObject(cmd.Context(), http.MethodPost, target, req)
			if err != nil {
				return err
			}
			defer func() { _ = resp.Body.Close() }()

			if resp.StatusCode != 200 {
				return fmt.Errorf("revoke certificate failed with status: %s", resp.Status)
			}

			fmt.Println("Certificate revoked successfully.")
			return nil
		},
	}

	cmd.Flags().StringVarP(&certType, "cert-type", "t", "", "Certificate type (e.g. x509, ssh)")
	cmd.Flags().StringVarP(&serialNumber, "serial-number", "s", "", "Serial number of the certificate to revoke")
	cmd.Flags().StringVarP(&subjectKeyID, "subject-key-id", "k", "", "Subject key ID of the certificate to revoke")
	cmd.Flags().StringVarP(&reason, "reason", "r", "", "Reason for revoking the certificate (optional)")
	return cmd
}
