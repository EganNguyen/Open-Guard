package secrets

import (
	"context"
	"fmt"
	"os"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/credentials"
	"github.com/aws/aws-sdk-go-v2/service/secretsmanager"
)

type Provider interface {
	GetSecret(ctx context.Context, secretID string) (string, error)
}

type EnvProvider struct{}

func (p *EnvProvider) GetSecret(ctx context.Context, secretID string) (string, error) {
	val := os.Getenv(secretID)
	if val == "" {
		return "", fmt.Errorf("environment variable %s not set", secretID)
	}
	return val, nil
}

type AWSProvider struct {
	client *secretsmanager.Client
}

func NewAWSProvider(ctx context.Context, endpoint, region, accessKey, secretKey string) (*AWSProvider, error) {
	customResolver := aws.EndpointResolverWithOptionsFunc(func(service, region string, options ...interface{}) (aws.Endpoint, error) {
		if endpoint != "" {
			return aws.Endpoint{
				URL:           endpoint,
				SigningRegion: region,
			}, nil
		}
		return aws.Endpoint{}, &aws.EndpointNotFoundError{}
	})

	cfg, err := config.LoadDefaultConfig(ctx,
		config.WithRegion(region),
		config.WithCredentialsProvider(credentials.NewStaticCredentialsProvider(accessKey, secretKey, "")),
		config.WithEndpointResolverWithOptions(customResolver),
	)
	if err != nil {
		return nil, err
	}

	return &AWSProvider{
		client: secretsmanager.NewFromConfig(cfg),
	}, nil
}

func (p *AWSProvider) GetSecret(ctx context.Context, secretID string) (string, error) {
	input := &secretsmanager.GetSecretValueInput{
		SecretId: aws.String(secretID),
	}

	result, err := p.client.GetSecretValue(ctx, input)
	if err != nil {
		return "", fmt.Errorf("failed to get secret %s from AWS: %w", secretID, err)
	}

	if result.SecretString != nil {
		return *result.SecretString, nil
	}

	return "", fmt.Errorf("secret %s has no string value", secretID)
}

func GetProvider(ctx context.Context) (Provider, error) {
	if os.Getenv("USE_AWS_SECRETS_MANAGER") == "true" {
		endpoint := os.Getenv("AWS_SECRETSMANAGER_ENDPOINT")
		region := os.Getenv("AWS_REGION")
		if region == "" {
			region = "us-east-1"
		}
		accessKey := os.Getenv("AWS_ACCESS_KEY_ID")
		secretKey := os.Getenv("AWS_SECRET_ACCESS_KEY")
		return NewAWSProvider(ctx, endpoint, region, accessKey, secretKey)
	}
	return &EnvProvider{}, nil
}
