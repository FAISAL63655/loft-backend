{
  "id": "loft-backend-df3i",
  "build": {
    "cgo_enabled": false
  },
  "cors": {
    "debug": true,
    "allow_headers": [
      "content-type",
      "authorization",
      "accept",
      "accept-language",
      "x-csrf-token",
      "x-timezone",
      "idempotency-key",
      "x-requested-with"
    ],
    "expose_headers": ["*"],
    "allow_origins_without_credentials": [
      "http://localhost:3000",
      "http://127.0.0.1:3000",
      "http://localhost:3001",
      "http://127.0.0.1:3001",
      "https://loft-frontend-chi.vercel.app",
      "https://loft-frontend-v3.vercel.app"
    ],
    "allow_origins_with_credentials": [
      "http://localhost:3000",
      "http://127.0.0.1:3000",
      "http://localhost:3001",
      "http://127.0.0.1:3001",
      "https://loft-frontend-chi.vercel.app",
      "https://admin.example.com",
      "https://loft-frontend-v3.vercel.app"
    ]
  }
}
