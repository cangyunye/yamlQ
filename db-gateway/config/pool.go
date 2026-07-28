package config

type PoolConfig struct {
	GlobalMaxOpen      int
	PerConnMaxOpen     int
	PerConnMaxIdle     int
	MaxConcurrentQuery int
	QueueDepth         int
	ConnMaxLifetimeSec int
	MaxRowsPerQuery    int
}

var CLI = PoolConfig{
	GlobalMaxOpen:      30,
	PerConnMaxOpen:     5,
	PerConnMaxIdle:     2,
	MaxConcurrentQuery: 10,
	QueueDepth:         20,
	ConnMaxLifetimeSec: 300,
	MaxRowsPerQuery:    10000,
}

var Service = PoolConfig{
	GlobalMaxOpen:      1000,
	PerConnMaxOpen:     50,
	PerConnMaxIdle:     10,
	MaxConcurrentQuery: 200,
	QueueDepth:         100,
	ConnMaxLifetimeSec: 3600,
	MaxRowsPerQuery:    100000,
}
