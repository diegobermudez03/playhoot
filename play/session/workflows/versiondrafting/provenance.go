package versiondrafting

import (
	sessionpkg "github.com/diegobermudez03/playhoot/play/session"
	"github.com/diegobermudez03/playhoot/play/session/workflows/versiondrafting/internal/changeattempt"
	internalrepo "github.com/diegobermudez03/playhoot/play/session/workflows/versiondrafting/internal/repo"
)

// written is the provenance of revision parent+1 of version, made by kind
// from origin.
func written(version *internalrepo.Version, kind string, parent int64, origin string) internalrepo.Provenance {
	return internalrepo.Provenance{
		Kind:           kind,
		Origin:         origin,
		Actor:          changeattempt.ActorOf(origin),
		ParentRevision: &parent,
		SDK:            version.ScriptProfile,
	}
}

// restoreProvenance is the provenance of a restore of revision from over
// parent.
func restoreProvenance(version *internalrepo.Version, parent int64, from int64, origin string) internalrepo.Provenance {
	provenance := written(version, sessionpkg.DraftRevisionRestore, parent, origin)
	provenance.RestoredFrom = &from
	return provenance
}

// publicRevision is a history entry in its public form.
func publicRevision(revision internalrepo.Revision) sessionpkg.DraftRevision {
	public := sessionpkg.DraftRevision{
		Number:       revision.Number,
		Kind:         revision.Kind,
		Origin:       revision.Origin,
		Actor:        revision.Actor,
		Parent:       revision.ParentRevision,
		RestoredFrom: revision.RestoredFrom,
		SDK:          revision.SDK,
		Files:        revision.Files,
		CreatedAt:    revision.CreatedAt,
	}
	if revision.FromVersionUUID != "" {
		public.From = &sessionpkg.DraftRevisionFrom{GameUUID: revision.FromGameUUID, VersionUUID: revision.FromVersionUUID, Revision: revision.FromRevision}
	}
	return public
}
