package main

import (
	"context"
	"dagger/aws-ecr-push/internal/dagger"
	"fmt"
)

type AwsEcrPush struct{}

func (ecr *AwsEcrPush) BuildAndPushOidc(ctx context.Context,
	// Path to a source directory
	src *Directory,
	// Path to a Dockerfile to build against
	// +optional
	// +default="Dockerfile"
	dockerfile string,
	// Path to a root context directory for the Dockerfile build
	// +optional
	// +default="./"
	context string,
	// OIDC token
	token *Secret,
	// AWS IAM Role to assume
	roleArn string,
	// The image name assigned to the container before uploading (should start with an ECR address and optionally include a :tag)
	tag string,
	// Session duration in seconds (min 900s/15min)
	// +optional
	// +default=900
	durationSec int,

	// AWS_DEFAULT_REGION
	// +optional
	// +default="us-east-1"
	region string,

	// Session name (will appear in logs and billing)
	// +optional
	sessionName string,
) (string, error) {
	return ecr.PublishOidc(ctx, ecr.BuildDockerfile(src, dockerfile, context), token, roleArn, tag, durationSec, region, sessionName)
}

func (ecr *AwsEcrPush) BuildAndPush(ctx context.Context,
	// Path to a source directory for the Dockerfile build
	src *Directory,
	// Path to a Dockerfile to build against
	// +optional
	// +default="Dockerfile"
	dockerfile string,
	// Path to a root context directory for the Dockerfile build
	// +optional
	// +default="./"
	context string,
	// AWS_ACCESS_KEY_ID
	keyId *Secret,
	// AWS_SECRET_ACCESS_KEY
	key *Secret,
	// AWS_SESSION_TOKEN
	token *Secret,
	// The image name assigned to the container before uploading (should start with an ECR address and optionally include a :tag)
	tag string,
	// AWS_DEFAULT_REGION
	// +optional
	// +default="us-east-1"
	region string,
) (string, error) {
	return ecr.Publish(ctx, ecr.BuildDockerfile(src, dockerfile, context), keyId, key, token, tag, region)
}

func (ecr *AwsEcrPush) PublishOidc(ctx context.Context,
	// Container to publish
	container *Container,
	// OIDC token
	token *Secret,
	// AWS IAM Role to assume
	roleArn string,
	// The image name assigned to the container before uploading (should start with an ECR address and optionally include a :tag)
	tag string,
	// Session duration in seconds (min 900s/15min)
	// +optional
	// +default=900
	durationSec int,
	// Default region
	// +optional
	// +default="us-east-1"
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
	return ecr.PublishContainer(ctx, container, tag, dag.SetSecret("ecrSecret", ecrSecret))
}

func (ecr *AwsEcrPush) Publish(ctx context.Context,
	// Container to publish
	container *Container,
	// AWS_ACCESS_KEY_ID
	keyId *Secret,
	// AWS_SECRET_ACCESS_KEY
	key *Secret,
	// AWS_SESSION_TOKEN
	token *Secret,
	// The image name assigned to the container before uploading (should start with an ECR address and optionally include a :tag)
	tag string,
	// Default region
	// +optional
	// +default="us-east-1"
	region string,
) (string, error) {
	secrets := dag.AwsOidcAuth().LoginSession(keyId, key, token, AwsOidcAuthLoginSessionOpts{Region: region})

	ecrSecret, err := secrets.Ecrsecret(ctx)
	if err != nil || ecrSecret == "" {
		fmt.Println("Failed to get ECR secret")
		return "", err
	}
	return ecr.PublishContainer(ctx, container, tag, dag.SetSecret("ecrSecret", ecrSecret))
}

func (ecr *AwsEcrPush) PublishContainer(ctx context.Context,
	container *Container,
	tag string,
	ecrSecret *Secret,
) (string, error) {
	return container.WithSecretVariable("registryPass", ecrSecret).
		WithRegistryAuth(tag, "AWS", ecrSecret).
		Publish(ctx, tag)
}

func (ecr *AwsEcrPush) BuildDockerfile(src *Directory,
	// +optional
	// +default="Dockerfile"
	dockerfile string,
	// Path to a root context directory for the Dockerfile build
	// +optional
	// +default="./"
	context string,
) *Container {
	return dag.Container().
		WithMountedDirectory("/src", src).
		Build(src.Directory(context), dagger.ContainerBuildOpts{Dockerfile: dockerfile})
}
