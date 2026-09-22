package auth

import "context"

type userIDContextKey struct{}

func ContextWithUserID(ctx context.Context, userID string) context.Context {
	return context.WithValue(ctx, userIDContextKey{}, userID)
}

func UserID(ctx context.Context) string {
	value, _ := ctx.Value(userIDContextKey{}).(string)
	return value
}
