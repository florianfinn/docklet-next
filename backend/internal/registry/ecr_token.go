// Source: getarcaneapp/arcane/backend/internal/registry/service.go
// Revision: 095cb778daba1034e2a39961199e712a75d4ba29
// Retrieved: 2026-11-06
// License: BSD-3-Clause
// Deviations: Extract ECR token request from persistence; inject bounded import HTTP client and retain existing update behavior.

package registry

import (
	"context"
	"encoding/base64"
	"fmt"
	"github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/credentials"
	"github.com/aws/aws-sdk-go-v2/service/ecr"
	"go.getarcane.app/sys/crypto"
	"net/http"
	"strings"
)

func requestECRTokenInternal(ctx context.Context, reg *ContainerRegistry, client *http.Client) (*ecrTokenResult, error) {
	// Decrypt the stored AWS secret access key.
	secretKey, decErr := crypto.Decrypt(reg.AWSSecretAccessKey)
	if decErr != nil {
		return nil, fmt.Errorf("failed to decrypt AWS secret key for registry %s: %w", reg.URL, decErr)
	}
	secretKey = strings.TrimSpace(secretKey)
	if secretKey == "" {
		return nil, fmt.Errorf("AWS secret access key is empty for registry %s", reg.URL)
	}

	// Call AWS ECR GetAuthorizationToken.
	options := []func(*config.LoadOptions) error{
		config.WithRegion(reg.AWSRegion),
		config.WithCredentialsProvider(credentials.NewStaticCredentialsProvider(
			reg.AWSAccessKeyID,
			secretKey,
			"",
		)),
	}
	if client != nil {
		options = append(options, config.WithHTTPClient(client), config.WithRetryMaxAttempts(1))
	}
	cfg, cfgErr := config.LoadDefaultConfig(ctx, options...)
	if cfgErr != nil {
		return nil, fmt.Errorf("failed to load AWS config for registry %s: %w", reg.URL, cfgErr)
	}

	ecrClient := ecr.NewFromConfig(cfg)
	result, ecrErr := ecrClient.GetAuthorizationToken(ctx, &ecr.GetAuthorizationTokenInput{})
	if ecrErr != nil {
		return nil, fmt.Errorf("failed to get ECR authorization token for registry %s: %w", reg.URL, ecrErr)
	}
	if len(result.AuthorizationData) == 0 || result.AuthorizationData[0].AuthorizationToken == nil {
		return nil, fmt.Errorf("ECR returned empty authorization data for registry %s", reg.URL)
	}

	// Decode base64 token → "AWS:<password>".
	decoded, decodeErr := base64.StdEncoding.DecodeString(*result.AuthorizationData[0].AuthorizationToken)
	if decodeErr != nil {
		return nil, fmt.Errorf("failed to decode ECR token for registry %s: %w", reg.URL, decodeErr)
	}
	parts := strings.SplitN(string(decoded), ":", 2)
	if len(parts) != 2 || parts[1] == "" {
		return nil, fmt.Errorf("unexpected ECR token format for registry %s", reg.URL)
	}
	ecrPassword := parts[1]

	return &ecrTokenResult{username: "AWS", password: ecrPassword}, nil
}
