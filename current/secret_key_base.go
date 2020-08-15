package current

import "context"

var secretKeyBase string

func SetSecretKeyBase(s string) {
	if secretKeyBase != "" {
		panic("cannot call SetSecretKeyBase twice")
	}
	if s == "" {
		panic("s must not be empty")
	}
	secretKeyBase = s
}

func SecretKeyBase(ctx context.Context) string {
	v := ctx.Value(secretKeyBaseCtxKey)
	if v != nil {
		return v.(string)
	}

	if secretKeyBase == "" {
		panic("missing SecretKeyBase in ctx and SecretKeyBase not set")
	}

	return secretKeyBase
}

func WithSecretKeyBase(ctx context.Context, s string) context.Context {
	return context.WithValue(ctx, secretKeyBaseCtxKey, s)
}
