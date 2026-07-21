-- Labels: the global label registry (initiative #95, self-hosted slice #113).
-- app.js keeps labels as a second array alongside notches -- a label is global
-- across the whole app, not owned by any one notch, and a notch's `tags` join to
-- it by name (see the LABELS note in app.js). The 0002 tables had nowhere to
-- store a label's identity or its color, so the single-user persistence slice
-- (#113) gives it a home here.
--
-- `color` is the auto-assigned palette swatch (a theme-aware CSS name like
-- 'red'/'amber'); `bg`/`fg` are NULL until a color picker is touched, at which
-- point they hold a fixed hex pair that overrides the palette, GitHub-label style.
CREATE TABLE labels (
    name  TEXT PRIMARY KEY,
    color TEXT NOT NULL DEFAULT '',
    bg    TEXT,                                  -- NULL, or a fixed hex override
    fg    TEXT                                   -- NULL, or a fixed hex override
);
