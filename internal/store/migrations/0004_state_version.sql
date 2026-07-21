-- Snapshot version for optimistic concurrency (#124).
--
-- The live build's /api/state seam is last-writer-wins per snapshot: a device's
-- dirty snapshot pushes up and whoever pushes last wins. That silently clobbers
-- the loser when two devices edit the same data offline and both reconnect.
--
-- This single-row table gives the stored snapshot a monotonic version, bumped on
-- every successful SaveState. PUT /api/state compares the version the client last
-- saw against this one (compare-and-swap): a stale base is rejected with 409 so
-- the conflict can be surfaced and resolved by hand instead of vanishing.
CREATE TABLE state_version (
	id      INTEGER PRIMARY KEY CHECK (id = 1),
	version INTEGER NOT NULL DEFAULT 0
);
INSERT INTO state_version (id, version) VALUES (1, 0);
