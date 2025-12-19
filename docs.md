.
├── cmd
│   └── api
│       └── main.go           <-- Entry point (initializes DB, Router, starts Server)
│
├── config
│   ├── config.go             <-- Load .env and struct for config
│   └── database.go           <-- DB Connection setup
│
├── internal                  <-- Application Logic (Private)
│   ├── models                <-- Shared Database Entities (GORM Structs)
│   │   ├── user.go
│   │   ├── job.go
│   │   ├── chat.go
│   │   └── contract.go
│   │
│   ├── middleware            <-- Auth, Logging, CORS, RateLimiting
│   │   └── auth.go
│   │
│   ├── modules               <-- DOMAIN DRIVEN DESIGN (The Scalable Part)
│   │   ├── auth              <-- Authentication Module
│   │   │   ├── handler.go    <-- Handles HTTP (Gin Context)
│   │   │   └── service.go    <-- Handles Login logic / JWT generation
│   │   │
│   │   ├── chat              <-- Chat Module
│   │   │   ├── handler_http.go
│   │   │   ├── handler_ws.go <-- WebSocket logic specific to chat
│   │   │   ├── service.go
│   │   │   └── repository.go <-- SQL queries for chat
│   │   │
│   │   ├── jobs              <-- Jobs Module
│   │   │   ├── handler.go
│   │   │   ├── service.go
│   │   │   └── repository.go
│   │   │
│   │   └── users             <-- User Management Module
│   │       ├── handler.go
│   │       └── service.go
│   │
│   └── server                <-- Server wiring
│       ├── router.go         <-- Register all module routes here
│       └── server.go         <-- Graceful shutdown logic
│
├── pkg                       <-- Public/Generic Libraries (Safe to reuse)
│   ├── logger
│   │   └── logger.go
│   ├── upload                <-- S3 / File upload logic
│   │   └── s3.go
│   └── utils
│       └── validation.go
│
├── media                     <-- Static files (Gitignored usually)
├── migrations                <-- SQL Migration files
├── go.mod
├── go.sum
└── Dockerfile