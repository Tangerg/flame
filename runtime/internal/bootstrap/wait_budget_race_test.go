//go:build race

package bootstrap

import "time"

const lifecycleWaitBudget = 60 * time.Second
