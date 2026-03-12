package config

import (
  "os"
  "strings"
)

func LoadEnv(path string) {
  data, err := os.ReadFile(path)
  if err != nil {
    return
  }

  lines := strings.Split(string(data), "\n")
  for _, line := range lines {
    line = strings.TrimSpace(line)
    if line == "" || strings.HasPrefix(line, "#") || strings.HasPrefix(line, ";") {
      continue
    }
    if strings.HasPrefix(line, "export ") {
      line = strings.TrimSpace(strings.TrimPrefix(line, "export "))
    }
    idx := strings.Index(line, "=")
    if idx <= 0 {
      continue
    }

    key := strings.TrimSpace(line[:idx])
    value := strings.TrimSpace(line[idx+1:])

    if len(value) >= 2 {
      if (value[0] == '"' && value[len(value)-1] == '"') || (value[0] == '\'' && value[len(value)-1] == '\'') {
        value = value[1 : len(value)-1]
      }
    }

    if _, exists := os.LookupEnv(key); !exists {
      _ = os.Setenv(key, value)
    }
  }
}
