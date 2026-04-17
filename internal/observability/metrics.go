package observability

import (
	"expvar"
	"sync/atomic"
	"time"
)

// Lightweight process metrics (no external deps).
// Can be later exposed via net/http/pprof or expvar endpoint if needed.

var (
	listProductsCount = expvar.NewInt("list_products_count")
	listProductsNs    = expvar.NewInt("list_products_total_ns")

	listColorsCount = expvar.NewInt("list_colors_count")
	listColorsNs    = expvar.NewInt("list_colors_total_ns")

	listSectionsCount = expvar.NewInt("list_sections_count")
	listSectionsNs    = expvar.NewInt("list_sections_total_ns")

	cacheHit  = expvar.NewInt("tree_cache_hit")
	cacheMiss = expvar.NewInt("tree_cache_miss")

	yadiskRequests = expvar.NewInt("yadisk_requests")
	yadiskRetries  = expvar.NewInt("yadisk_retries")

	tgSends   = expvar.NewInt("telegram_sends")
	tgRetries = expvar.NewInt("telegram_retries")
)

func ObserveListProducts(d time.Duration) {
	listProductsCount.Add(1)
	listProductsNs.Add(d.Nanoseconds())
}

func ObserveListColors(d time.Duration) {
	listColorsCount.Add(1)
	listColorsNs.Add(d.Nanoseconds())
}

func ObserveListSections(d time.Duration) {
	listSectionsCount.Add(1)
	listSectionsNs.Add(d.Nanoseconds())
}

func CacheHit()  { cacheHit.Add(1) }
func CacheMiss() { cacheMiss.Add(1) }

func YadiskRequest() { yadiskRequests.Add(1) }
func YadiskRetry()   { yadiskRetries.Add(1) }

func TelegramSend()  { tgSends.Add(1) }
func TelegramRetry() { tgRetries.Add(1) }

// For unit tests / internal checks.
var _ = atomic.Int64{}
