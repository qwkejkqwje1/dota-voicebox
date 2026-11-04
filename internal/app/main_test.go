package app

import (
	"os"
	"testing"
)

// Тесты проверяют русские строки интерфейса — язык системы (LANG на сборочном сервере) на них не влияет.
func TestMain(m *testing.M) {
	os.Setenv("LC_ALL", "ru_RU.UTF-8")
	os.Exit(m.Run())
}
