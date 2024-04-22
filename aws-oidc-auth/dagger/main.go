package main

import (
	"bufio"
	"context"
	"fmt"
	"regexp"
	"strconv"
	"strings"
)

type AwsOidcAuth struct{}

type AwsSecrets struct {
	DurationSec     int
	FromTsUtc       int
	UntilTsUtc      int
	DefaultRegion   string
	OIDCToken       string
	AccessKeyId     string
	SecretAccessKey string
	SessionToken    string
	ECRSecret       string
}

func (aws *AwsOidcAuth) LoginOidc(
	ctx context.Context,

	// OIDC token
	token string,

	// AWS IAM Role to assume
	roleArn string,

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
) (*AwsSecrets, error) {

	if sessionName == "" {
		sessionName = fmt.Sprintf("OIDC_LOGIN-%s", region)
	}

	exported, err := aws.authenticate(ctx, token, region, roleArn, sessionName, durationSec)
	if err != nil {
		return nil, err
	}

	raw := aws.readFileValues(exported)

	secrets, err := aws.toSecrets(raw)
	if err != nil {
		return nil, err
	}

	return secrets, nil
}

func (aws *AwsOidcAuth) toSecrets(raw map[string]string) (*AwsSecrets, error) {
	duration, err := strconv.Atoi(raw["AWS_SESSION_DURATION"])
	if err != nil {
		fmt.Println("Cannot find 'AWS_SESSION_DURATION'")
		return nil, err
	}
	iss, err := strconv.Atoi(raw["AWS_SESSION_ISS_UTC"])
	if err != nil {
		fmt.Println("Cannot find 'AWS_SESSION_ISS_UTC'")
		return nil, err
	}
	exp, err := strconv.Atoi(raw["AWS_SESSION_EXP_UTC"])
	if err != nil {
		fmt.Println("Cannot find 'AWS_SESSION_EXP_UTC'")
		return nil, err
	}
	return &AwsSecrets{
		DurationSec:     duration,
		FromTsUtc:       iss,
		UntilTsUtc:      exp,
		DefaultRegion:   raw["AWS_DEFAULT_REGION"],
		OIDCToken:       raw["OIDC_TOKEN"],
		AccessKeyId:     raw["AWS_ACCESS_KEY_ID"],
		SecretAccessKey: raw["AWS_SECRET_ACCESS_KEY"],
		SessionToken:    raw["AWS_SESSION_TOKEN"],
		ECRSecret:       raw["AWS_ECR_SECRET"],
	}, nil
}

func (aws *AwsOidcAuth) authenticate(ctx context.Context, token string, region string, roleArn string, sessionName string, durationSec int) (string, error) {
	credsFilePath := "/tmp/aws_creds"

	exported := dag.Container().From("public.ecr.aws/aws-cli/aws-cli").
		WithoutEntrypoint().
		WithEnvVariable("CREDS_FILE_PATH", credsFilePath).
		WithExec([]string{"bash", "-c", "env"}).
		WithExec([]string{"bash", "-ec",
			fmt.Sprintf(`echo '
				export OIDC_TOKEN="%s"
				export AWS_DEFAULT_REGION="%s"
				export AWS_ROLE_ARN="%s"
				export AWS_SESSION_NAME="%s"
				export AWS_SESSION_DURATION="%d"
				' > "${CREDS_FILE_PATH}"`, token, region, roleArn, sessionName, durationSec)}).
		WithExec([]string{"bash", "-ec",
			`. "${CREDS_FILE_PATH}"
			ts=$(TZ=UTC date +%s)
			ts_exp=$((${ts} + ${AWS_SESSION_DURATION}))
			echo "export AWS_SESSION_ISS_UTC=${ts}" >>"${CREDS_FILE_PATH}"
			echo "export AWS_SESSION_EXP_UTC=${ts_exp}" >>"${CREDS_FILE_PATH}"`,
		}).
		WithExec([]string{"bash", "-ec",
			`. "${CREDS_FILE_PATH}"
			AWS_CREDS=$(aws sts assume-role-with-web-identity \
      		--role-arn "${AWS_ROLE_ARN:?Assumed role ARN missing}" \
      		--role-session-name "${AWS_SESSION_NAME}" \
      		--web-identity-token "${OIDC_TOKEN?OIDC Token missing}" \
		  	--duration-seconds ${AWS_SESSION_DURATION:-901} \
		  	--query 'Credentials.[AccessKeyId,SecretAccessKey,SessionToken]' \
		  	--output text)
			printf '
				export AWS_ACCESS_KEY_ID="%s"
				export AWS_SECRET_ACCESS_KEY="%s"
				export AWS_SESSION_TOKEN="%s"
			' \
      		${AWS_CREDS:?AWS credentials missing} \
			>> "${CREDS_FILE_PATH}"`,
		}).
		WithExec([]string{"bash", "-ec",
			`. "${CREDS_FILE_PATH}" 
			printf '
				export AWS_ECR_SECRET="%s"
			' \
			$(aws ecr get-login-password --region "${AWS_DEFAULT_REGION}") >> "${CREDS_FILE_PATH}"`}).
		File(credsFilePath)
	return exported.Contents(ctx)
}

func (aws *AwsOidcAuth) readFileValues(contents string) map[string]string {
	pattern := regexp.MustCompile("^(?P<prefix>.*\\s+)?(?P<key>[a-zA-Z0-9_]+)\\s*=\\s*\"?(?P<value>[^\"]*)\"?$")
	scanner := bufio.NewScanner(strings.NewReader(contents))

	raw := make(map[string]string)

	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" {
			continue
		}
		result := aws.extractGroups(pattern, line)
		if len(result) > 0 {
			raw[result["key"]] = result["value"]
		}
	}
	return raw
}

func (aws *AwsOidcAuth) extractGroups(pattern *regexp.Regexp, line string) map[string]string {
	result := make(map[string]string)
	if pattern.MatchString(line) {
		match := pattern.FindStringSubmatch(line)
		for i, name := range pattern.SubexpNames() {
			if i != 0 && name != "" {
				result[name] = match[i]
			}
		}

	}
	return result
}
