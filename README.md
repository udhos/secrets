[![license](http://img.shields.io/badge/license-MIT-blue.svg)](https://github.com/udhos/secrets/blob/main/LICENSE)
[![Go Report Card](https://goreportcard.com/badge/github.com/udhos/secrets)](https://goreportcard.com/report/github.com/udhos/secrets)
[![Go Reference](https://pkg.go.dev/badge/github.com/udhos/secrets.svg)](https://pkg.go.dev/github.com/udhos/secrets)
[![Artifact Hub](https://img.shields.io/endpoint?url=https://artifacthub.io/badge/repository/secrets-proxy)](https://artifacthub.io/packages/search?repo=secrets-proxy)
[![Docker Pulls](https://img.shields.io/docker/pulls/udhos/secrets)](https://hub.docker.com/r/udhos/secrets)

# secrets

[secrets](https://github.com/udhos/secrets) is a service to retrieve secrets from multiple secrets sources by using the generic secret name defined in the project [boilerplate](https://github.com/udhos/boilerplate).

# TODO

- [X] slog
- [X] slog level config
- [X] review metrics
- [X] review traces

# Test

```
# Start kind: kind create cluster --name lab

./scripts/run-secrets.sh

curl -d '{"secret_name":"aws-parameterstore:sa-east-1:/microservice9/mongodb:uri"}' localhost:8080/secret
```

# Test Vault

1. Run vault server with permission to AWS STS

```
key=$(echo $(grep aws_access_key_id ~/.aws/credentials | awk -F= '{print$2}'))
secret=$(echo $(grep aws_secret_access_key ~/.aws/credentials | awk -F= '{print$2}'))

docker run --rm -p 8200:8200 \
    -e 'VAULT_DEV_ROOT_TOKEN_ID=dev-only-token' \
    -e AWS_ACCESS_KEY_ID=$key \
    -e AWS_SECRET_ACCESS_KEY=$secret \
    hashicorp/vault:1.17.5
```

2. Use vault cli to configure the server

```
CLIENT_IAM_ROLE_ARN=... ;# fill this with client role arn

export VAULT_ADDR=http://127.0.0.1:8200

vault login

# enable aws auth
vault auth enable aws

# create policy for role dev-role-iam
vault policy write "example-policy" -<<EOF
path "secret/*" {
  capabilities = ["create", "read"]
}
EOF

# create role dev-role-iam
vault write \
  auth/aws/role/dev-role-iam \
  auth_type=iam \
  policies=example-policy \
  max_ttl=24h \
  bound_iam_principal_arn=$CLIENT_IAM_ROLE_ARN

# Put a secret to query later
vault kv put secret/myapp1 mongodb='{"uri":"abc"}'
```

3. Run `secrets` service

NOTE: Vault client sdk has some limitations: (1) It does no support AWS_PROFILE. (2) It does not support credential files (`~/.aws/credentials`). It only worked with aws env vars (role credentials from env vars).

```
# Login into $CLIENT_IAM_ROLE_ARN with `aws sts assume-role` and put values into env vars.

# Start kind: kind create cluster --name lab

# Then run:

export KUBEGROUP_FORCE_NAMESPACE_DEFAULT=true

secrets
```

4. Test with curl

```bash
curl -d '{"secret_name":"vault::,dev-role-iam,http,localhost,8200,secret/myapp1/mongodb:uri"}' localhost:8080/secret

{"secret_value":"abc","status":"200"}
```