package main

import (
  "log"
  "os"

  "domainguard/internal/config"
  "domainguard/internal/database"
)

const (
  defaultConnString = "Data Source=(localdb)\\MSSQLLocalDB;Integrated Security=True;Persist Security Info=False;Pooling=False;MultipleActiveResultSets=False;Encrypt=True;TrustServerCertificate=False;Application Name=\"DomainGuard\";Command Timeout=0"
  defaultDBName     = "DomainGuardDB"
)

func main() {
  config.LoadEnv(".env")

  connString := defaultConnString
  if value := os.Getenv("DB_CONN_STRING"); value != "" {
    connString = value
  }

  dbName := defaultDBName
  if value := os.Getenv("DB_NAME"); value != "" {
    dbName = value
  }

  db, err := database.Connect(connString, dbName)
  if err != nil {
    log.Fatalf("failed to connect database: %v", err)
  }
  defer db.Close()

  if err := database.EnsureTables(db); err != nil {
    log.Fatalf("failed to ensure tables: %v", err)
  }

  log.Printf("tables ensured in database %s", dbName)
}
