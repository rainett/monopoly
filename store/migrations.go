package store

const schema = `
CREATE TABLE IF NOT EXISTS users (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    username TEXT UNIQUE NOT NULL,
    password_hash TEXT NOT NULL,
    created_at DATETIME DEFAULT CURRENT_TIMESTAMP
);

CREATE TABLE IF NOT EXISTS sessions (
    session_id TEXT PRIMARY KEY,
    user_id INTEGER NOT NULL,
    created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
    expires_at DATETIME NOT NULL,
    FOREIGN KEY (user_id) REFERENCES users(id) ON DELETE CASCADE
);

CREATE TABLE IF NOT EXISTS games (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    status TEXT NOT NULL DEFAULT 'waiting',
    created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
    max_players INTEGER DEFAULT 4
);

CREATE TABLE IF NOT EXISTS game_players (
    game_id INTEGER NOT NULL,
    user_id INTEGER NOT NULL,
    player_order INTEGER NOT NULL,
    is_ready INTEGER DEFAULT 0,
    is_current_turn INTEGER DEFAULT 0,
    has_played_turn INTEGER DEFAULT 0,
    money INTEGER DEFAULT 1500,
    position INTEGER DEFAULT 0,
    is_bankrupt INTEGER DEFAULT 0,
    has_rolled INTEGER DEFAULT 0,
    pending_action TEXT DEFAULT '',
    in_jail INTEGER DEFAULT 0,
    jail_turns INTEGER DEFAULT 0,
    PRIMARY KEY (game_id, user_id),
    FOREIGN KEY (game_id) REFERENCES games(id),
    FOREIGN KEY (user_id) REFERENCES users(id)
);

CREATE TABLE IF NOT EXISTS game_properties (
    game_id INTEGER NOT NULL,
    position INTEGER NOT NULL,
    owner_id INTEGER NOT NULL,
    is_mortgaged INTEGER DEFAULT 0,
    PRIMARY KEY (game_id, position),
    FOREIGN KEY (game_id) REFERENCES games(id),
    FOREIGN KEY (owner_id) REFERENCES users(id)
);

CREATE INDEX IF NOT EXISTS idx_sessions_user_id ON sessions(user_id);
CREATE INDEX IF NOT EXISTS idx_sessions_expires_at ON sessions(expires_at);
CREATE INDEX IF NOT EXISTS idx_games_status ON games(status);
CREATE INDEX IF NOT EXISTS idx_game_players_game_id ON game_players(game_id);
CREATE INDEX IF NOT EXISTS idx_game_players_user_id ON game_players(user_id);
CREATE INDEX IF NOT EXISTS idx_game_players_current_turn ON game_players(game_id, is_current_turn);
CREATE INDEX IF NOT EXISTS idx_game_properties_game_id ON game_properties(game_id);
CREATE INDEX IF NOT EXISTS idx_game_properties_owner ON game_properties(game_id, owner_id);

CREATE TABLE IF NOT EXISTS game_improvements (
    game_id INTEGER NOT NULL,
    position INTEGER NOT NULL,
    count INTEGER NOT NULL DEFAULT 0,
    PRIMARY KEY (game_id, position),
    FOREIGN KEY (game_id) REFERENCES games(id) ON DELETE CASCADE
);

CREATE TABLE IF NOT EXISTS game_card_decks (
    game_id INTEGER NOT NULL,
    deck_type TEXT NOT NULL,
    card_order TEXT NOT NULL,
    next_index INTEGER DEFAULT 0,
    PRIMARY KEY (game_id, deck_type),
    FOREIGN KEY (game_id) REFERENCES games(id) ON DELETE CASCADE
);

CREATE TABLE IF NOT EXISTS player_jail_cards (
    game_id INTEGER NOT NULL,
    user_id INTEGER NOT NULL,
    deck_type TEXT NOT NULL,
    PRIMARY KEY (game_id, user_id, deck_type),
    FOREIGN KEY (game_id) REFERENCES games(id) ON DELETE CASCADE,
    FOREIGN KEY (user_id) REFERENCES users(id) ON DELETE CASCADE
);

CREATE TABLE IF NOT EXISTS game_trades (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    game_id INTEGER NOT NULL,
    from_user_id INTEGER NOT NULL,
    to_user_id INTEGER NOT NULL,
    offer_json TEXT NOT NULL,
    status TEXT NOT NULL DEFAULT 'pending',
    created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
    FOREIGN KEY (game_id) REFERENCES games(id) ON DELETE CASCADE,
    FOREIGN KEY (from_user_id) REFERENCES users(id) ON DELETE CASCADE,
    FOREIGN KEY (to_user_id) REFERENCES users(id) ON DELETE CASCADE
);

CREATE INDEX IF NOT EXISTS idx_game_trades_game_id ON game_trades(game_id);
CREATE INDEX IF NOT EXISTS idx_game_trades_status ON game_trades(game_id, status);

CREATE TABLE IF NOT EXISTS friendships (
    user_id_1 INTEGER NOT NULL,
    user_id_2 INTEGER NOT NULL,
    status TEXT NOT NULL DEFAULT 'pending',
    created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
    PRIMARY KEY (user_id_1, user_id_2),
    FOREIGN KEY (user_id_1) REFERENCES users(id) ON DELETE CASCADE,
    FOREIGN KEY (user_id_2) REFERENCES users(id) ON DELETE CASCADE
);

CREATE INDEX IF NOT EXISTS idx_friendships_user1 ON friendships(user_id_1);
CREATE INDEX IF NOT EXISTS idx_friendships_user2 ON friendships(user_id_2);
CREATE INDEX IF NOT EXISTS idx_friendships_status ON friendships(status);

CREATE TABLE IF NOT EXISTS game_invites (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    game_id INTEGER NOT NULL,
    from_user_id INTEGER NOT NULL,
    to_user_id INTEGER NOT NULL,
    status TEXT NOT NULL DEFAULT 'pending',
    created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
    FOREIGN KEY (game_id) REFERENCES games(id) ON DELETE CASCADE,
    FOREIGN KEY (from_user_id) REFERENCES users(id) ON DELETE CASCADE,
    FOREIGN KEY (to_user_id) REFERENCES users(id) ON DELETE CASCADE,
    UNIQUE(game_id, to_user_id)
);

CREATE INDEX IF NOT EXISTS idx_game_invites_to_user ON game_invites(to_user_id, status);
`
