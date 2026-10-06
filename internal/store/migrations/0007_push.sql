-- Web push: the daemon's VAPID key pair and every device's subscription.
--
-- The key pair is generated on first use and kept, since a subscription is
-- made against the public key and a new pair would orphan every device. One
-- row, enforced by its id.
CREATE TABLE vapid (
    id      INTEGER PRIMARY KEY CHECK (id = 1),
    public  TEXT NOT NULL CHECK (public <> ''),
    private TEXT NOT NULL CHECK (private <> ''),
    created TEXT NOT NULL
);

-- A subscription belongs to a device and is posted by it on every page load,
-- which is how the device learns that it lapsed. lapsed is when a push
-- service answered 404 or 410 for it, '' while it has not.
CREATE TABLE subscriptions (
    id        INTEGER PRIMARY KEY,
    endpoint  TEXT NOT NULL UNIQUE CHECK (endpoint <> ''),
    p256dh    TEXT NOT NULL CHECK (p256dh <> ''),
    auth      TEXT NOT NULL CHECK (auth <> ''),
    created   TEXT NOT NULL,
    confirmed TEXT NOT NULL,
    lapsed    TEXT NOT NULL DEFAULT ''
);
