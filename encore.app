{
  "id": "loft-backend-df3i",
  "build": {
    "cgo_enabled": false
  },
  "cors": {
    "debug": true,
    "allow_headers": [
      "Content-Type",
      "Authorization",
      "Accept",
      "Accept-Language",
      "X-CSRF-Token",
      "X-Timezone",
      "Idempotency-Key",
      "X-Requested-With"
    ],
    "expose_headers": ["*"],
    "allow_origins_without_credentials": [
      "http://localhost:3000",
      "http://127.0.0.1:3000",
      "http://localhost:3001",
      "http://127.0.0.1:3001",
      "https://loft-frontend-chi.vercel.app",
    ],
    "allow_origins_with_credentials": [
      "http://localhost:3000",
      "http://127.0.0.1:3000",
      "http://localhost:3001",
      "http://127.0.0.1:3001",
      "https://loft-frontend-chi.vercel.app",
      "https://admin.example.com"
    ]
  }
}
