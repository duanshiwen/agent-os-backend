package service

import (
	"errors"
	"fmt"
)

const SyncSchemaVersion = 1

const (
	SyncObjectMessage          = "message"
	SyncObjectConversation     = "conversation"
	SyncObjectParticipant      = "participant"
	SyncObjectMessageReaction  = "message_reaction"
	SyncObjectConversationRead = "conversation_read"
	SyncObjectKnowledge        = "knowledge"
	SyncObjectSkill            = "skill"
	SyncObjectAgent            = "agent"
	SyncObjectServer           = "server"
	SyncObjectPlugin           = "plugin"
	SyncObjectProfile          = "profile"
)

const (
	SyncOperationCreated           = "created"
	SyncOperationUpdated           = "updated"
	SyncOperationDeleted           = "deleted"
	SyncOperationAdded             = "added"
	SyncOperationRemoved           = "removed"
	SyncOperationEnabled           = "enabled"
	SyncOperationDisabled          = "disabled"
	SyncOperationInstalled         = "installed"
	SyncOperationUninstalled       = "uninstalled"
	SyncOperationPermissionGranted = "permission_granted"
	SyncOperationPermissionRevoked = "permission_revoked"
	SyncOperationRead              = "read"
	SyncOperationReactionAdded     = "reaction_added"
	SyncOperationReactionRemoved   = "reaction_removed"
)

// Compatibility aliases kept while the rest of the backend still uses the
// original event/action names.
const (
	SyncEventMessage          = SyncObjectMessage
	SyncEventConversation     = SyncObjectConversation
	SyncEventParticipant      = SyncObjectParticipant
	SyncEventMessageReaction  = SyncObjectMessageReaction
	SyncEventConversationRead = SyncObjectConversationRead
	SyncEventKnowledge        = SyncObjectKnowledge
	SyncEventSkill            = SyncObjectSkill
	SyncEventAgent            = SyncObjectAgent
	SyncEventServer           = SyncObjectServer
	SyncEventPlugin           = SyncObjectPlugin
	SyncEventProfile          = SyncObjectProfile

	SyncActionCreated           = SyncOperationCreated
	SyncActionUpdated           = SyncOperationUpdated
	SyncActionDeleted           = SyncOperationDeleted
	SyncActionAdded             = SyncOperationAdded
	SyncActionRemoved           = SyncOperationRemoved
	SyncActionEnabled           = SyncOperationEnabled
	SyncActionDisabled          = SyncOperationDisabled
	SyncActionInstalled         = SyncOperationInstalled
	SyncActionUninstalled       = SyncOperationUninstalled
	SyncActionPermissionGranted = SyncOperationPermissionGranted
	SyncActionPermissionRevoked = SyncOperationPermissionRevoked
	SyncActionRead              = SyncOperationRead
	SyncActionReactionAdded     = SyncOperationReactionAdded
	SyncActionReactionRemoved   = SyncOperationReactionRemoved
)

var (
	ErrUnsupportedSyncEvent    = errors.New("unsupported sync event")
	ErrSyncIdempotencyConflict = errors.New("sync idempotency conflict")
)

var supportedSyncOperations = map[string]map[string]bool{
	SyncObjectMessage: {
		SyncOperationCreated:         true,
		SyncOperationUpdated:         true,
		SyncOperationDeleted:         true,
		SyncOperationReactionAdded:   true,
		SyncOperationReactionRemoved: true,
	},
	SyncObjectConversation: {
		SyncOperationCreated: true,
		SyncOperationUpdated: true,
		SyncOperationRead:    true,
	},
	SyncObjectParticipant: {
		SyncOperationAdded:   true,
		SyncOperationUpdated: true,
		SyncOperationRemoved: true,
	},
	SyncObjectMessageReaction: {
		SyncOperationAdded:   true,
		SyncOperationRemoved: true,
	},
	SyncObjectConversationRead: {
		SyncOperationUpdated: true,
	},
	SyncObjectKnowledge: {
		SyncOperationCreated: true,
		SyncOperationUpdated: true,
		SyncOperationDeleted: true,
	},
	SyncObjectSkill: {
		SyncOperationEnabled:     true,
		SyncOperationDisabled:    true,
		SyncOperationUpdated:     true,
		SyncOperationInstalled:   true,
		SyncOperationUninstalled: true,
	},
	SyncObjectAgent: {
		SyncOperationUpdated: true,
	},
	SyncObjectServer: {
		SyncOperationAdded:   true,
		SyncOperationUpdated: true,
		SyncOperationRemoved: true,
	},
	SyncObjectPlugin: {
		SyncOperationAdded:             true,
		SyncOperationUpdated:           true,
		SyncOperationRemoved:           true,
		SyncOperationInstalled:         true,
		SyncOperationUninstalled:       true,
		SyncOperationEnabled:           true,
		SyncOperationDisabled:          true,
		SyncOperationPermissionGranted: true,
		SyncOperationPermissionRevoked: true,
	},
	SyncObjectProfile: {
		SyncOperationUpdated: true,
	},
}

func BuildSyncEventType(objectType, operation string) (string, error) {
	if err := ValidateSyncEvent(objectType, operation); err != nil {
		return "", err
	}
	return objectType + "." + operation, nil
}

func ValidateSyncEvent(objectType, operation string) error {
	allowedOperations, ok := supportedSyncOperations[objectType]
	if !ok {
		return fmt.Errorf("%w: unsupported object type %s", ErrUnsupportedSyncEvent, objectType)
	}
	if !allowedOperations[operation] {
		return fmt.Errorf("%w: unsupported operation %s for object type %s", ErrUnsupportedSyncEvent, operation, objectType)
	}
	return nil
}
