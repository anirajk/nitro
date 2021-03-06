// Copyright (c) 2017 Couchbase, Inc.
// Licensed under the Apache License, Version 2.0 (the "License"); you may not use this file
// except in compliance with the License. You may obtain a copy of the License at
//   http://www.apache.org/licenses/LICENSE-2.0
// Unless required by applicable law or agreed to in writing, software distributed under the
// License is distributed on an "AS IS" BASIS, WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND,
// either express or implied. See the License for the specific language governing permissions
// and limitations under the License.

package plasma

import (
	"github.com/couchbase/nitro/skiplist"
	"runtime"
	"unsafe"
)

type Config struct {
	MaxDeltaChainLen   int
	MaxPageItems       int
	MinPageItems       int
	MaxPageLSSSegments int
	Compare            skiplist.CompareFn
	ItemSize           ItemSizeFn
	CopyItem           ItemCopyFn
	ItemRunSize        ItemRunSizeFn
	CopyItemRun        ItemRunCopyFn

	IndexKeySize ItemSizeFn
	CopyIndexKey ItemCopyFn

	LSSLogSegmentSize   int64
	File                string
	FlushBufferSize     int
	NumPersistorThreads int
	NumEvictorThreads   int

	LSSCleanerThreshold       int
	LSSCleanerMaxThreshold    int
	LSSCleanerMinSize         int64
	LSSCleanerThrottleMinSize int64

	AutoLSSCleaning bool
	AutoSwapper     bool

	EnableShapshots bool

	TriggerSwapper func(SwapperContext) bool
	shouldPersist  bool

	MaxSnSyncFrequency int
	SyncInterval       int

	UseMemoryMgmt bool
	UseMmap       bool

	UseCompression bool
}

func applyConfigDefaults(cfg Config) Config {
	if cfg.NumPersistorThreads == 0 {
		cfg.NumPersistorThreads = runtime.NumCPU()
	}

	if cfg.NumEvictorThreads == 0 {
		cfg.NumEvictorThreads = runtime.NumCPU()
	}

	if cfg.TriggerSwapper == nil {
		cfg.TriggerSwapper = QuotaSwapper
	}

	if cfg.File == "" {
		cfg.AutoLSSCleaning = false
		cfg.AutoSwapper = false
	} else {
		cfg.shouldPersist = true
	}

	if cfg.MaxSnSyncFrequency == 0 {
		cfg.MaxSnSyncFrequency = 360000
	}

	if cfg.LSSLogSegmentSize == 0 {
		cfg.LSSLogSegmentSize = 1024 * 1024 * 1024 * 4
	}

	if cfg.MaxPageLSSSegments == 0 {
		cfg.MaxPageLSSSegments = 4
	}

	if cfg.CopyItem == nil {
		cfg.CopyItem = memcopy
	}

	if cfg.CopyIndexKey == nil {
		cfg.CopyIndexKey = memcopy
	}

	if cfg.IndexKeySize == nil {
		cfg.IndexKeySize = cfg.ItemSize
	}

	if cfg.ItemRunSize == nil {
		cfg.ItemRunSize = func(srcItms []unsafe.Pointer) uintptr {
			var sz uintptr
			for _, itm := range srcItms {
				sz += cfg.ItemSize(itm)
			}

			return sz
		}

		cfg.CopyItemRun = func(srcItms, dstItms []unsafe.Pointer, data unsafe.Pointer) {
			var offset uintptr
			for i, itm := range srcItms {
				dstItm := unsafe.Pointer(uintptr(data) + offset)
				sz := cfg.ItemSize(itm)
				cfg.CopyItem(dstItm, itm, int(sz))
				dstItms[i] = dstItm
				offset += sz
			}
		}
	}

	if cfg.LSSCleanerMaxThreshold == 0 {
		cfg.LSSCleanerMaxThreshold = cfg.LSSCleanerThreshold + 10
	}

	if cfg.LSSCleanerThrottleMinSize < cfg.LSSCleanerMinSize {
		cfg.LSSCleanerThrottleMinSize = cfg.LSSCleanerMinSize
	}

	return cfg
}

func DefaultConfig() Config {
	return Config{
		UseCompression:   true,
		MaxDeltaChainLen: 200,
		MaxPageItems:     400,
		MinPageItems:     25,
		Compare:          cmpItem,
		ItemSize: func(itm unsafe.Pointer) uintptr {
			if itm == skiplist.MinItem || itm == skiplist.MaxItem {
				return 0
			}
			return uintptr((*item)(itm).Size())
		},
		CopyItem:                  copyItem,
		CopyIndexKey:              copyItem,
		FlushBufferSize:           1024 * 1024 * 1,
		LSSCleanerThreshold:       10,
		LSSCleanerMinSize:         16 * 1024 * 1024,
		LSSCleanerThrottleMinSize: 1024 * 1024 * 1024,
		AutoLSSCleaning:           true,
		AutoSwapper:               false,
		EnableShapshots:           true,
		SyncInterval:              0,
	}
}
