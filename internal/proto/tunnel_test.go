package proto

import "testing"

func TestTunnelRegisterRequestValidate(t *testing.T) {
	tests := []struct {
		name    string
		req     TunnelRegisterRequest
		wantErr bool
	}{
		{
			name: "valid single subdomain",
			req: TunnelRegisterRequest{
				Type:       "tunnel_register",
				Subdomains: []string{"my-app"},
			},
			wantErr: false,
		},
		{
			name: "valid multiple subdomains",
			req: TunnelRegisterRequest{
				Type:       "tunnel_register",
				Subdomains: []string{"test-app", "my-app", "api"},
			},
			wantErr: false,
		},
		{
			name: "invalid type",
			req: TunnelRegisterRequest{
				Type:       "invalid",
				Subdomains: []string{"my-app"},
			},
			wantErr: true,
		},
		{
			name: "empty subdomains list",
			req: TunnelRegisterRequest{
				Type:       "tunnel_register",
				Subdomains: []string{},
			},
			wantErr: true,
		},
		{
			name: "nil subdomains",
			req: TunnelRegisterRequest{
				Type:       "tunnel_register",
				Subdomains: nil,
			},
			wantErr: true,
		},
		{
			name: "empty subdomain string",
			req: TunnelRegisterRequest{
				Type:       "tunnel_register",
				Subdomains: []string{"valid", ""},
			},
			wantErr: true,
		},
		{
			name: "invalid subdomain character",
			req: TunnelRegisterRequest{
				Type:       "tunnel_register",
				Subdomains: []string{"my_app!"},
			},
			wantErr: true,
		},
		{
			name: "duplicate subdomains",
			req: TunnelRegisterRequest{
				Type:       "tunnel_register",
				Subdomains: []string{"my-app", "my-app"},
			},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.req.Validate()
			if (err != nil) != tt.wantErr {
				t.Errorf("TunnelRegisterRequest.Validate() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}
