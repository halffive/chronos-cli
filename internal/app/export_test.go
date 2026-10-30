package app

import "context"

func ReadHidden(ctx context.Context, read func() (string, error), restore func()) (string, error) {
	return readHidden(ctx, hiddenRead{read: read, restore: restore})
}
