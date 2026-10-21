package main

import (
	"log"
	"os"
	"strconv"
	"time"
)

// batch — заглушка «ночной аналитики»: в бесконечном цикле жрёт CPU и память,
// не делая ничего полезного. Потребление настраивается через переменные окружения,
// чтобы в Частях 6–7 можно было создавать нехватку ресурсов и давление по памяти.
func main() {
	cpuWorkers := getenvInt("CPU_WORKERS", 1)
	memMB := getenvInt("MEMORY_MB", 256)
	growthMB := getenvInt("MEMORY_GROWTH_MB", 0)
	maxMB := getenvInt("MEMORY_MAX_MB", 0)

	// Жжём CPU в плотных циклах: по одному воркеру на ядро.
	for w := 0; w < cpuWorkers; w++ {
		go func() {
			var x uint64 = 1
			for {
				x = x*6364136223846793005 + 1442695040888963407
				_ = x
			}
		}()
	}

	// Держим память резидентной (страницы реально трогаем, чтобы они не ушли в swap).
	held := make([][]byte, 0, 64)
	hold := func(mb int) {
		buf := make([]byte, mb*1024*1024)
		for i := 0; i < len(buf); i += 4096 {
			buf[i] = 1
		}
		held = append(held, buf)
		log.Printf("batch: holding %d MiB total", totalMB(held))
	}

	hold(memMB)

	// Опционально растём по памяти, чтобы спровоцировать давление на ноде.
	if growthMB > 0 {
		ticker := time.NewTicker(time.Second)
		defer ticker.Stop()
		for range ticker.C {
			total := totalMB(held)
			if maxMB > 0 && total+growthMB > maxMB {
				continue
			}
			hold(growthMB)
		}
	} else {
		select {}
	}
}

func totalMB(held [][]byte) int {
	total := 0
	for _, b := range held {
		total += len(b) / (1024 * 1024)
	}
	return total
}

func getenvInt(key string, def int) int {
	if v := os.Getenv(key); v != "" {
		if n, err := strconv.Atoi(v); err == nil {
			return n
		}
	}
	return def
}