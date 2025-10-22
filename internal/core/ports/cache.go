package ports

import "time"

// Cache defines the interface for caching operations
type Cache interface {
	// Get retrieves a value from cache
	Get(key string) (interface{}, bool)
	
	// Set stores a value in cache with default expiration
	Set(key string, value interface{})
	
	// SetWithExpiration stores a value with custom expiration
	SetWithExpiration(key string, value interface{}, expiration time.Duration)
	
	// Delete removes a value from cache
	Delete(key string)
	
	// Clear removes all values from cache
	Clear()
	
	// ItemCount returns the number of items in cache
	ItemCount() int
}
