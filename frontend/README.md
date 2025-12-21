# Codenames Svelte Frontend

This directory contains the modern Svelte 5 frontend for Codenames. It replaces the legacy Next.js implementation with a lighter, faster, and more reactive Single Page Application (SPA).

## 🚀 Tech Stack

- **Framework**: [SvelteKit](https://kit.svelte.dev/) (SPA mode with `adapter-static`)
- **State Management**: Svelte 5 Runes (`$state`, `$derived`)
- **Styling**: [Tailwind CSS v4](https://tailwindcss.com/)
- **Build Tool**: Vite

## 📂 Project Structure

```
src/
├── lib/
│   ├── api.ts            # REST API client wrapper
│   ├── game.svelte.ts    # Global GameStore (Auth, WS, State)
│   ├── types.ts          # Shared TypeScript interfaces (mirrors Go structs)
│   └── components/       # UI Components
│       ├── BoardView.svelte   # Main gameplay grid
│       ├── Card.svelte        # Individual card component
│       ├── GameControls.svelte# Spymaster inputs / Turn actions
│       ├── GameInfo.svelte    # Scoreboard and Turn indicator
│       └── SetupView.svelte   # Role assignment lobby
├── routes/
│   ├── +page.svelte      # Login screen
│   ├── lobby/            # Game browser
│   └── game/[id]/        # Main game container
└── app.html              # Root HTML template
```

## 🛠 Development

### Prerequisites

1.  **Go Backend**: The frontend expects the API server to be running on port 8080.

    ```bash
    # From project root
    go run ./cmd/codenames-server --addr :8080
    ```

2.  **Node.js**: Ensure Node.js is installed.

### Running the Frontend

```bash
cd web/svelte
npm install
npm run dev
```

The app will be available at `http://localhost:5173`.

## 🏗 Architecture

### State Management (`game.svelte.ts`)

We use a global singleton `GameStore` class powered by Svelte 5 Runes. It handles:

- **User Identity**: Persisted/Restored via API.
- **WebSocket Connection**: Automatically connects/reconnects when entering a game.
- **Game State**: Reactive updates from WS messages (`GAME_START`, `CLUE_GIVEN`, `GUESS_GIVEN`).

### API Layer

- **REST**: `api.ts` handles all HTTP commands (Join, Start, Clue, Guess).
- **Real-time**: WebSockets push state changes. The store applies these changes directly to the reactive `game` object, instantly updating the UI.

### Styling

- Visuals use **Tailwind CSS**.
- **Spymaster View**: Sees all card colors (tinted/muted).
- **Operative View**: Sees neutral cards until they are revealed.
- **Responsive**: Grid adjusts for mobile/desktop.

## 📦 Building for Production

Since this is an SPA, it builds to static HTML/JS/CSS assets.

```bash
npm run build
```

Output is generated in the `build/` directory. These files should be served by a web server (like Nginx) which also proxies `/api` traffic to the Go backend.
