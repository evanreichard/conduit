package store

import (
	"context"
)

type contextKey struct{}

var recordIDKey = contextKey{}

func withRecord(ctx context.Context, rec *TunnelRecord) context.Context {
	return context.WithValue(ctx, recordIDKey, rec)
}

func getRecord(ctx context.Context) (*TunnelRecord, bool) {
	id, ok := ctx.Value(recordIDKey).(*TunnelRecord)
	return id, ok
}
