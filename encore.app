{
  "id": "loft-backend-df3i",
  "build": {
    "cgo_enabled": false
  },
  "cors": {
    "debug": true,
    "allow_headers": [
      // أضفنا الشكلين لحساسية بعض المزودين لحالة الأحرف
      "authorization", "Authorization",
      "content-type", "Content-Type",
      "accept", "Accept",
      "accept-language", "Accept-Language",
      "x-csrf-token", "X-CSRF-Token",
      "x-timezone", "X-Timezone",
      "idempotency-key", "Idempotency-Key",
      "x-requested-with", "X-Requested-With"
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
