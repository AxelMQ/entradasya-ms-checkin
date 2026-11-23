# ms-checkin: Microservicio de Control de Acceso y Aforo en Puerta

Microservicio perimetral de control de acceso e ingreso a eventos para la plataforma **EntradasYa**. Valida el código escaneado por el personal de puerta (`portero`) contra `ms-tickets`, previene condiciones de carrera y garantiza el principio de uso único de entradas mediante restricciones a nivel de motor en PostgreSQL (`checkin_db`), registrando auditoría en tiempo real y estadísticas de aforo.

* **Tecnología:** Go 1.22+ / Gin Framework / `pgx/v5`
* **Puerto:** `8007`
* **Base de datos:** `checkin_db` (PostgreSQL 16)
* **Autenticación:** Descentralizada vía JWT simétrico (HS256) con RBAC (`portero`, `organizador`, `admin`)
* **Comunicación Interna:** Cliente HTTP hacia `ms-tickets` (`:8005`) con cabecera `X-Internal-Token`

---

## 1. Arquitectura y Estructura de Paquetes

```text
backend/ms-checkin/
├── cmd/
│   └── api/
│       └── main.go                 # Punto de entrada y arranque del servidor Gin
├── internal/
│   ├── client/
│   │   ├── tickets_client.go       # Cliente HTTP hacia ms-tickets (:8005)
│   │   └── tickets_client_test.go  # Pruebas unitarias con httptest
│   ├── config/
│   │   └── config.go               # Carga y validación de variables de entorno
│   ├── database/
│   │   └── postgres.go             # Conexión pgxpool y wrapper Pinger
│   ├── handler/
│   │   ├── checkin_handler.go      # Handlers: scan, stats, history
│   │   ├── checkin_handler_test.go # Pruebas unitarias de handlers HTTP con httptest
│   │   └── health_handler.go       # Probes de liveness y readiness
│   ├── middleware/
│   │   ├── auth.go                 # Validación local JWT HS256 y RBAC
│   │   ├── auth_test.go            # Pruebas unitarias de seguridad y roles
│   │   └── logger.go               # Logger de peticiones y correlation ID
│   ├── model/
│   │   └── checkin.go              # Entidades y DTOs de admisión y aforo
│   ├── repository/
│   │   ├── checkin_repo.go         # Implementación PostgreSQL con pgxpool
│   │   └── mock_repo.go            # Mock en memoria seguro para tests
│   └── service/
│       ├── checkin_service.go      # Reglas de admisión, prevención de doble uso y auditoría
│       └── checkin_service_test.go # Pruebas unitarias de la lógica de negocio
├── migrations/
│   └── 001_create_checkins_table.sql # DDL de tabla checkins e índice único condicional
├── .env.example
├── .env
├── README.md
├── go.mod
└── go.sum
```

---

## 2. Variables de Entorno

| Variable | Descripción | Valor por Defecto |
| :--- | :--- | :--- |
| `PORT` | Puerto HTTP del servicio | `8007` |
| `DATABASE_URL` | Cadena de conexión PostgreSQL | `postgres://checkin_user:checkin_pass@localhost:5432/checkin_db?sslmode=disable` |
| `JWT_SECRET` | Clave secreta simétrica HS256 compartida | `super-secure-shared-secret-entradasya-2026` |
| `SERVICES_TICKETS_URL` | URL base de `ms-tickets` | `http://localhost:8005` |
| `INTERNAL_TOKEN` | Token para comunicación inter-servicios | `internal-service-secret-entradasya-2026` |
| `LOG_LEVEL` | Nivel de logs (`debug`, `info`, `warn`, `error`) | `info` |

---

## 3. Especificación de Endpoints

### 3.1 Escanear Entrada en Puerta
* **Método y Ruta:** `POST /api/v1/checkin/scan`
* **Autorización:** `Bearer <JWT>` con rol `portero`
* **Request:**
  ```json
  {
    "ticket_code": "EY-ROCK-8X4N2Q",
    "event_id": "b1a2c3d4-e5f6-7a8b-9c0d-1e2f3a4b5c6d"
  }
  ```
* **Respuestas:**
  - `200 OK`: Acceso permitido (`ACCEPTED`).
  - `409 Conflict`: Entrada ya utilizada previamente (`REJECTED`, `ALREADY_USED`).
  - `400 Bad Request`: Entrada de otro evento (`REJECTED`, `EVENT_MISMATCH`).
  - `404 Not Found`: Código inexistente (`REJECTED`, `INVALID_CODE`).
  - `401 Unauthorized`: Token ausente o inválido.
  - `403 Forbidden`: Rol insuficiente.

### 3.2 Estadísticas de Aforo en Vivo
* **Método y Ruta:** `GET /api/v1/checkin/events/:eventId/stats`
* **Autorización:** `Bearer <JWT>` con rol `portero`, `organizador` o `admin`
* **Respuesta (`200 OK`):**
  ```json
  {
    "event_id": "b1a2c3d4-e5f6-7a8b-9c0d-1e2f3a4b5c6d",
    "total_checked_in": 342,
    "by_sector": [
      { "sector_name": "General", "checked_in": 250 },
      { "sector_name": "VIP", "checked_in": 92 }
    ],
    "last_checkin_at": "2026-12-15T20:45:12Z"
  }
  ```

### 3.3 Historial Reciente de Escaneos
* **Método y Ruta:** `GET /api/v1/checkin/events/:eventId/history?limit=20`
* **Autorización:** `Bearer <JWT>` con rol `portero`, `organizador` o `admin`

### 3.4 Health Checks
* `GET /health/live`: Liveness probe (`200 OK` si el servicio está en ejecución).
* `GET /health/ready`: Readiness probe (`200 OK` si `checkin_db` responde al ping; `503 Service Unavailable` si la base de datos está caída).

---

## 4. Pruebas y Compilación

Ejecutar todas las pruebas unitarias y de integración:
```bash
go test -v ./...
```

Compilar el binario:
```bash
go build ./cmd/api
```

Iniciar el microservicio:
```bash
go run ./cmd/api/main.go
```

## Estado de Verificacion CI/CD

- Pipeline `.github/workflows/ci-develop.yml` verificado con disparo automatico en rama `develop`, generando artifacts descargables e imagenes en GHCR y Docker Hub.
