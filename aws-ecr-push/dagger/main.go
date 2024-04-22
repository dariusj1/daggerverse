package main

import (
	"context"
	"dagger/aws-ecr-push/internal/dagger"
	"fmt"
)

type AwsEcrPush struct{}

func (ecr *AwsEcrPush) BuildAndPushOidc(ctx context.Context,
	// Path to a root context directory for the Dockerfile build
	root *Directory,
	// Path to a Dockerfile to build against
	// +optional
	// +default "Dockerfile"
	dockerfile string,
	// OIDC token
	token string,
	// AWS IAM Role to assume
	roleArn string,
	// The image name assigned to the container before uploading (should start with an ECR address and optionally include a :tag)
	tag string,
	// Session duration in seconds (min 900s/15min)
	// +optional
	// +default 900
	durationSec int,

	// Default region
	// +optional
	// +default "us-east-1"
	region string,

	// Session name (will appear in logs and billing)
	// +optional
	sessionName string,
) (string, error) {
	return ecr.PublishOidc(ctx, ecr.BuildDockerfile(root, dockerfile), token, roleArn, tag, durationSec, region, sessionName)
}

func (ecr *AwsEcrPush) PublishOidc(ctx context.Context,
	// Container to publish
	container *Container,
	// OIDC token
	token string,
	// AWS IAM Role to assume
	roleArn string,
	// The image name assigned to the container before uploading (should start with an ECR address and optionally include a :tag)
	tag string,
	// Session duration in seconds (min 900s/15min)
	// +optional
	// +default 900
	durationSec int,
	// Default region
	// +optional
	// +default "us-east-1"
	region string,
	// Session name (will appear in logs and billing)
	// +optional
	sessionName string,
) (string, error) {
	secrets := dag.AwsOidcAuth().LoginOidc(token, roleArn, AwsOidcAuthLoginOidcOpts{
		DurationSec: durationSec,
		Region:      region,
		SessionName: sessionName,
	})

	ecrSecret, err := secrets.Ecrsecret(ctx)
	if err != nil || ecrSecret == "" {
		fmt.Println("Failed to get ECR secret")
		return "", err
	}
	return ecr.PublishContainer(ctx, container, tag, ecrSecret)
}

func (ecr *AwsEcrPush) PublishContainer(ctx context.Context,
	container *Container,
	tag string,
	ecrSecret string,
) (string, error) {
	dag.SetSecret("registryPass", ecrSecret)
	return container.WithRegistryAuth(tag, "AWS", dag.Secret("registryPass", dagger.SecretOpts{})).
		Publish(ctx, tag)
}

func (ecr *AwsEcrPush) BuildDockerfile(root *Directory,
	// +optional
	// +default "Dockerfile"
	dockerfile string,
) *Container {
	return dag.Container().Build(root, dagger.ContainerBuildOpts{Dockerfile: dockerfile})
}