package tencent

import (
	"context"
	"fmt"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/tencentcloud/tencentcloud-sdk-go-intl-en/tencentcloud/common"
)

// LoadStaticCredentials reads a Secret that holds literal TENCENTCLOUD_SECRET_ID /
// TENCENTCLOUD_SECRET_KEY. Wraps the values in the SDK's common.Credential so
// they satisfy CredentialIface.
func LoadStaticCredentials(ctx context.Context, c client.Client, name, namespace string) (common.CredentialIface, error) {
	var s corev1.Secret
	if err := c.Get(ctx, types.NamespacedName{Name: name, Namespace: namespace}, &s); err != nil {
		return nil, fmt.Errorf("get secret: %w", err)
	}
	sid := string(s.Data["secret-id"])
	skey := string(s.Data["secret-key"])
	if sid == "" || skey == "" {
		return nil, fmt.Errorf("secret %s/%s missing secret-id or secret-key", namespace, name)
	}
	return common.NewCredential(sid, skey), nil
}

// BuildCredentialProvider returns the first credential resolved from the
// chain: TKE pod-identity (env: TKE_REGION / TKE_PROVIDER_ID /
// TKE_WEB_IDENTITY_TOKEN_FILE / TKE_ROLE_ARN) → STS env vars
// (TENCENTCLOUD_SECRET_ID / TENCENTCLOUD_SECRET_KEY) → CVM instance metadata
// role. Each provider is the SDK's own; we just compose them with
// common.NewProviderChain. // ponytail: SDK ships all three providers; the
// chain is the only thing we add.
func BuildCredentialProvider(ctx context.Context) (common.CredentialIface, error) {
	var providers []common.Provider
	if p, err := common.DefaultTkeOIDCRoleArnProvider(); err == nil {
		providers = append(providers, p)
	}
	providers = append(providers, common.DefaultEnvProvider())
	providers = append(providers, common.DefaultCvmRoleProvider())
	cred, err := common.NewProviderChain(providers).GetCredential()
	if err != nil {
		return nil, fmt.Errorf("tencent: credential chain exhausted: %w", err)
	}
	return cred, nil
}
