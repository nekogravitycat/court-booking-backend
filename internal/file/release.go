package file

import (
	"context"
	"log"
)

// ReleaseReplaced best-effort deletes a file whose reference was just replaced or cleared.
// Call it only after the new reference is durably stored. keepID is the new reference
// ("" when the reference was cleared); the old file is left alone if it is still in use.
func ReleaseReplaced(ctx context.Context, svc Service, oldID *string, keepID string) {
	if oldID == nil || *oldID == "" || *oldID == keepID {
		return
	}
	if err := svc.Delete(ctx, *oldID); err != nil {
		log.Printf("warning: failed to delete file %s: %v", *oldID, err)
	}
}
