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
	"fmt"
	"github.com/couchbase/nitro/skiplist"
	"os"
	"runtime"
	"runtime/debug"
	"sync"
	"testing"
	"time"
	"unsafe"
)

var testCfg = Config{
	MaxDeltaChainLen: 200,
	MaxPageItems:     400,
	MinPageItems:     25,
	Compare:          skiplist.CompareInt,
	ItemSize: func(x unsafe.Pointer) uintptr {
		if x == skiplist.MinItem || x == skiplist.MaxItem {
			return 0
		}
		return unsafe.Sizeof(new(skiplist.IntKeyItem))
	},
	File:                   "teststore.data",
	FlushBufferSize:        1024 * 1024,
	LSSCleanerMaxThreshold: 100,
	LSSCleanerThreshold:    10,
	AutoLSSCleaning:        false,
}

func newTestIntPlasmaStore(cfg Config) *Plasma {
	s, err := New(cfg)
	if err != nil {
		panic(err)
	}

	return s
}

func TestPlasmaSimple(t *testing.T) {
	os.RemoveAll("teststore.data")
	s := newTestIntPlasmaStore(testCfg)
	defer s.Close()

	w := s.NewWriter()
	for i := 0; i < 1000000; i++ {
		w.Insert(skiplist.NewIntKeyItem(i))
	}

	for i := 0; i < 1000000; i++ {
		itm := skiplist.NewIntKeyItem(i)
		got, _ := w.Lookup(itm)
		if skiplist.CompareInt(itm, got) != 0 {
			t.Errorf("mismatch %d != %d", i, skiplist.IntFromItem(got))
		}
	}

	for i := 0; i < 800000; i++ {
		w.Delete(skiplist.NewIntKeyItem(i))
	}

	for i := 0; i < 1000000; i++ {
		itm := skiplist.NewIntKeyItem(i)
		got, _ := w.Lookup(itm)
		if i < 800000 {
			if got != nil {
				t.Errorf("Expected missing %d", i)
			}
		} else {
			if skiplist.CompareInt(itm, got) != 0 {
				t.Errorf("Expected %d, got %d", i, skiplist.IntFromItem(got))
			}
		}
	}

	fmt.Println(s.GetStats())
}

func doInsert(w *Writer, wg *sync.WaitGroup, id, n int) {
	defer wg.Done()

	for i := 0; i < n; i++ {
		val := i + id*n
		itm := skiplist.NewIntKeyItem(val)
		w.Insert(itm)
	}
}

func doUpdate(w *Writer, wg *sync.WaitGroup, id, n int) {
	defer wg.Done()

	for i := 0; i < n; i++ {
		val := i + id*n
		itm := skiplist.NewIntKeyItem(val)
		w.Delete(itm)
		w.Insert(itm)
	}
}

func doDelete(w *Writer, wg *sync.WaitGroup, id, n int) {
	defer wg.Done()

	for i := 0; i < n; i++ {
		val := i + id*n
		itm := skiplist.NewIntKeyItem(val)
		w.Delete(itm)
	}
}

func doLookup(w *Writer, wg *sync.WaitGroup, id, n int) {
	defer wg.Done()

	for i := 0; i < n; i++ {
		val := i + id*n
		itm := skiplist.NewIntKeyItem(val)
		if x, _ := w.Lookup(itm); x == nil {
			panic(i)
		}
	}
}

func TestPlasmaInsertPerf(t *testing.T) {
	var wg sync.WaitGroup

	os.RemoveAll("teststore.data")
	numThreads := 8
	n := 20000000
	nPerThr := n / numThreads
	s := newTestIntPlasmaStore(testCfg)
	defer s.Close()
	total := numThreads * nPerThr

	t0 := time.Now()
	for i := 0; i < numThreads; i++ {
		wg.Add(1)
		w := s.NewWriter()
		go doInsert(w, &wg, i, nPerThr)
	}
	wg.Wait()

	dur := time.Since(t0)

	fmt.Println(s.GetStats())
	fmt.Printf("%d items insert took %v -> %v items/s\n", total, dur, float64(total)/float64(dur.Seconds()))
}

func TestPlasmaDeletePerf(t *testing.T) {
	var wg sync.WaitGroup

	os.RemoveAll("teststore.data")
	numThreads := 8
	n := 20000000
	nPerThr := n / numThreads
	s := newTestIntPlasmaStore(testCfg)
	defer s.Close()
	total := numThreads * nPerThr

	for i := 0; i < numThreads; i++ {
		wg.Add(1)
		w := s.NewWriter()
		go doInsert(w, &wg, i, nPerThr)
	}
	wg.Wait()
	fmt.Println(s.GetStats())

	t0 := time.Now()
	for i := 0; i < numThreads; i++ {
		wg.Add(1)
		w := s.NewWriter()
		go doDelete(w, &wg, i, nPerThr)
	}
	wg.Wait()

	dur := time.Since(t0)

	fmt.Println(s.GetStats())
	fmt.Printf("%d items delete took %v -> %v items/s\n", total, dur, float64(total)/float64(dur.Seconds()))
}

func TestPlasmaLookupPerf(t *testing.T) {
	var wg sync.WaitGroup

	os.RemoveAll("teststore.data")
	numThreads := 8
	n := 20000000
	nPerThr := n / numThreads
	s := newTestIntPlasmaStore(testCfg)
	defer s.Close()
	total := numThreads * nPerThr

	for i := 0; i < numThreads; i++ {
		wg.Add(1)
		w := s.NewWriter()
		go doInsert(w, &wg, i, nPerThr)
	}
	wg.Wait()

	t0 := time.Now()
	for i := 0; i < numThreads; i++ {
		wg.Add(1)
		w := s.NewWriter()
		go doLookup(w, &wg, i, nPerThr)
	}
	wg.Wait()

	dur := time.Since(t0)

	fmt.Println(s.GetStats())
	fmt.Printf("%d items lookup took %v -> %v items/s\n", total, dur, float64(total)/float64(dur.Seconds()))
}

func TestIteratorSimple(t *testing.T) {
	os.RemoveAll("teststore.data")
	s := newTestIntPlasmaStore(testCfg)
	defer s.Close()
	w := s.NewWriter()
	for i := 0; i < 1000000; i++ {
		w.Insert(skiplist.NewIntKeyItem(i))
	}

	i := 0
	itr := s.NewIterator()
	for itr.SeekFirst(); itr.Valid(); itr.Next() {
		if v := skiplist.IntFromItem(itr.Get()); v != i {
			t.Errorf("expected %d, got %d", i, v)
		}

		i++
	}

	if i != 1000000 {
		t.Errorf("expected %d, got %d", 1000000, i)
	}

}

func TestIteratorSeek(t *testing.T) {
	os.RemoveAll("teststore.data")
	s := newTestIntPlasmaStore(testCfg)
	defer s.Close()
	w := s.NewWriter()
	for i := 0; i < 1000000; i++ {
		w.Insert(skiplist.NewIntKeyItem(i))
	}

	itr := s.NewIterator().(*Iterator)
	for i := 0; i < 1000000; i++ {
		itr.Seek(skiplist.NewIntKeyItem(i))
		if itr.Valid() {
			if v := skiplist.IntFromItem(itr.Get()); v != i {
				t.Errorf("expected %d, got %d", i, v)
			}
		} else {
			t.Errorf("%d not found", i)
		}
	}
}

func TestPlasmaIteratorLookupPerf(t *testing.T) {
	var wg sync.WaitGroup

	os.RemoveAll("teststore.data")
	numThreads := 8
	n := 20000000
	nPerThr := n / numThreads
	s := newTestIntPlasmaStore(testCfg)
	defer s.Close()
	total := numThreads * nPerThr

	for i := 0; i < numThreads; i++ {
		wg.Add(1)
		w := s.NewWriter()
		go doInsert(w, &wg, i, nPerThr)
	}
	wg.Wait()

	t0 := time.Now()
	for i := 0; i < numThreads; i++ {
		wg.Add(1)
		go func(wg *sync.WaitGroup, id, n int) {
			defer wg.Done()
			itr := s.NewIterator()

			for i := 0; i < n; i++ {
				val := i + id*n
				itm := skiplist.NewIntKeyItem(val)
				itr.Seek(itm)
			}
		}(&wg, i, nPerThr)
	}
	wg.Wait()

	dur := time.Since(t0)

	fmt.Println(s.GetStats())
	fmt.Printf("%d items iterator seek took %v -> %v items/s\n", total, dur, float64(total)/float64(dur.Seconds()))
}

func TestPlasmaPersistor(t *testing.T) {
	os.RemoveAll("teststore.data")
	s := newTestIntPlasmaStore(testCfg)
	defer s.Close()
	w := s.NewWriter()
	for i := 0; i < 18000000; i++ {
		w.Insert(skiplist.NewIntKeyItem(i * 10))
	}

	t0 := time.Now()
	s.PersistAll()
	fmt.Println("took", time.Since(t0), s.lss.UsedSpace())
	for i := 0; i < 2000000; i++ {
		w.Insert(skiplist.NewIntKeyItem(5 + i*100))
	}
	t0 = time.Now()
	s.PersistAll()
	fmt.Println("took", time.Since(t0), s.lss.UsedSpace())
	for i := 0; i < 2000000; i++ {
		w.Insert(skiplist.NewIntKeyItem(6 + i*100))
	}
	t0 = time.Now()
	s.PersistAll()
	fmt.Println("took", time.Since(t0), s.lss.UsedSpace())
	fmt.Println(s.GetStats())
}

func TestPlasmaRecoverySimple(t *testing.T) {
	os.RemoveAll("teststore.data")
	s := newTestIntPlasmaStore(testCfg)

	w := s.NewWriter()
	for i := 0; i < 10; i++ {
		w.Insert(skiplist.NewIntKeyItem(i))
	}
	s.PersistAll()
	s.Close()

	s = newTestIntPlasmaStore(testCfg)
	defer s.Close()

	itr := s.NewIterator()
	count := 0
	for itr.SeekFirst(); itr.Valid(); itr.Next() {
		v := skiplist.IntFromItem(itr.Get())
		if count != v {
			t.Errorf("expected %d, got %d", count, v)
		}

		count++
	}
}

func TestPlasmaRecovery(t *testing.T) {
	var wg sync.WaitGroup
	os.RemoveAll("teststore.data")
	s := newTestIntPlasmaStore(testCfg)

	numThreads := 8
	n := 1000000
	m := 900000
	ws := make([]*Writer, numThreads)
	for i := 0; i < numThreads; i++ {
		wg.Add(1)
		w := s.NewWriter()
		ws[i] = w
		go doInsert(w, &wg, i, n/numThreads)
	}
	wg.Wait()

	for i := 0; i < numThreads; i++ {
		wg.Add(1)
		w := ws[i]
		go doDelete(w, &wg, i, m/numThreads)
	}
	wg.Wait()

	fmt.Println(s.GetStats())
	fmt.Println(s.Skiplist.GetStats())
	s.PersistAll()
	s.Close()

	fmt.Println("***** reopen file *****")
	s = newTestIntPlasmaStore(testCfg)
	w := s.NewWriter()
	fmt.Println(s.GetStats())
	fmt.Println(s.Skiplist.GetStats())

	for i := 0; i < m; i++ {
		itm := skiplist.NewIntKeyItem(i)
		got, _ := w.Lookup(itm)
		if got != nil {
			t.Errorf("expected nil %v", skiplist.IntFromItem(got))
		}
	}

	for i := m; i < n; i++ {
		itm := skiplist.NewIntKeyItem(i)
		got, _ := w.Lookup(itm)
		if got == nil {
			t.Errorf("mismatch %d != nil", i)
		} else if skiplist.CompareInt(itm, got) != 0 {
			t.Errorf("mismatch %d != %d", i, skiplist.IntFromItem(got))
		}
	}

	itr := s.NewIterator()
	count := 0
	for itr.SeekFirst(); itr.Valid(); itr.Next() {
		v := skiplist.IntFromItem(itr.Get())
		if count+m != v {
			t.Errorf("Validation: expected %d, got %d", count+m, v)
		}
		count++
	}

	if count != n-m {
		t.Errorf("Expected %d, got %d", n-m, count)
	}
	s.Close()
}

func TestPlasmaLSSCleaner(t *testing.T) {
	os.RemoveAll("teststore.data")
	cfg := testCfg
	cfg.LSSCleanerThreshold = 10
	s := newTestIntPlasmaStore(cfg)
	defer s.Close()

	w := s.NewWriter()

	for i := 0; i < 10000000; i++ {
		w.Insert(skiplist.NewIntKeyItem(i))
	}

	_, ds0, used0 := s.GetLSSInfo()
	s.PersistAll()
	fmt.Println(s.GetStats())
	fmt.Println()

	donech := make(chan bool)

	go func() {
		for {
			select {
			case <-donech:
				return
			default:
			}
			s.PersistAll()
			time.Sleep(time.Second * time.Duration(5))
		}
	}()

	go s.lssCleanerDaemon()
	for x := 0; x < 20; x++ {
		fmt.Println("Running iteration..", x)
		for i := 0; i < 1000000; i++ {
			itm := skiplist.NewIntKeyItem(i)
			w.Delete(itm)
			w.Insert(itm)
		}

		fmt.Println(s.GetStats())
	}

	time.Sleep(time.Second)
	frag, ds, used := s.GetLSSInfo()

	fmt.Printf("LSSInfo: frag:%d, ds:%d, used:%d\n", frag, ds, used)
	if used > used0*110/100 || ds > ds0*110/100 {
		t.Errorf("Expected better cleaning with frag ~ 10%%")
	}

	donech <- true
	s.stoplssgc <- struct{}{}
	<-s.stoplssgc
}

func TestPlasmaCleanerPerf(t *testing.T) {
	var wg sync.WaitGroup

	os.RemoveAll("teststore.data")
	numThreads := 8
	n := 100000000
	nPerThr := n / numThreads
	cfg := testCfg
	cfg.LSSCleanerThreshold = 10
	cfg.AutoLSSCleaning = true
	s := newTestIntPlasmaStore(cfg)
	defer s.Close()
	total := numThreads * nPerThr

	t0 := time.Now()
	for i := 0; i < numThreads; i++ {
		wg.Add(1)
		w := s.NewWriter()
		go doInsert(w, &wg, i, nPerThr)
	}
	wg.Wait()

	dur := time.Since(t0)
	fmt.Printf("%d items insert took %v -> %v items/s\n", total, dur, float64(total)/float64(dur.Seconds()))
	s.PersistAll()

	t0 = time.Now()
	for i := 0; i < numThreads; i++ {
		wg.Add(1)
		w := s.NewWriter()
		go doUpdate(w, &wg, i, nPerThr)
	}
	wg.Wait()
	dur = time.Since(t0)
	fmt.Printf("%d items update took %v -> %v items/s\n", total, dur, float64(total)/float64(dur.Seconds()))

	fmt.Println(s.GetStats())

	frag, ds, used := s.GetLSSInfo()

	fmt.Printf("LSSInfo: frag:%d, ds:%d, used:%d\n", frag, ds, used)

}

func TestPlasmaIteratorSwapin(t *testing.T) {
	os.RemoveAll("teststore.data")
	s := newTestIntPlasmaStore(testCfg)
	defer s.Close()

	n := 1000000
	w := s.NewWriter()
	for i := 0; i < n; i++ {
		if i%100 != 0 {
			w.Insert(skiplist.NewIntKeyItem(i))
		}
	}

	s.EvictAll()

	for i := 0; i < n; i++ {
		if i%100 == 0 {
			w.Insert(skiplist.NewIntKeyItem(i))
		}
	}

	itr := s.NewIterator()
	c := 0
	for itr.SeekFirst(); itr.Valid(); itr.Next() {
		c++
	}

	if c != n {
		t.Errorf("Expected %d items, got %d", n, c)
	}

	miss := s.GetStats().CacheMisses
	if miss != 4901 {
		t.Errorf("Expected cache miss=4901, but got %d", miss)
	}
}

func TestPlasmaEviction(t *testing.T) {
	os.RemoveAll("teststore.data")
	s := newTestIntPlasmaStore(testCfg)
	defer s.Close()

	n := 1000000
	w := s.NewWriter()
	for i := 0; i < n+100000; i++ {
		w.Insert(skiplist.NewIntKeyItem(i))
	}

	for i := n; i < n+100000; i++ {
		w.Delete(skiplist.NewIntKeyItem(i))
	}

	w.CompactAll()

	s.EvictAll()
	s.EvictAll()
	mem := s.GetStats().MemSz

	if mem != 0 {
		t.Errorf("Expected memory_usage=0, got=%d", mem)
	}

	for i := 0; i < n; i++ {
		itm := skiplist.NewIntKeyItem(i)
		got, _ := w.Lookup(itm)
		if skiplist.CompareInt(itm, got) != 0 {
			t.Errorf("mismatch %d != %d", i, skiplist.IntFromItem(got))
		}
	}

	for i := n; i < n+1000000; i++ {
		w.Insert(skiplist.NewIntKeyItem(i))
	}

	for i := n; i < n+100000; i++ {
		w.Delete(skiplist.NewIntKeyItem(i))
	}

	s.EvictAll()
	mem = s.GetStats().MemSz

	if mem != 0 {
		t.Errorf("Expected memory_usage=0, got=%d", mem)
	}

}

func TestPlasmaEvictPerf(t *testing.T) {
	var wg sync.WaitGroup

	os.RemoveAll("teststore.data")
	numThreads := 8
	n := 100000000
	nPerThr := n / numThreads
	cfg := testCfg
	cfg.LSSCleanerThreshold = 10
	cfg.AutoLSSCleaning = false
	s := newTestIntPlasmaStore(cfg)
	defer s.Close()
	total := numThreads * nPerThr

	t0 := time.Now()
	for i := 0; i < numThreads; i++ {
		wg.Add(1)
		w := s.NewWriter()
		go doInsert(w, &wg, i, nPerThr)
	}
	wg.Wait()

	dur := time.Since(t0)
	fmt.Printf("%d items insert took %v -> %v items/s\n", total, dur, float64(total)/float64(dur.Seconds()))
	s.PersistAll()

	s.EvictAll()
	runtime.GC()
	debug.FreeOSMemory()

	fmt.Println("Evicted items...")

	t0 = time.Now()
	for i := 0; i < numThreads; i++ {
		wg.Add(1)
		w := s.NewWriter()
		go doLookup(w, &wg, i, nPerThr)
	}
	wg.Wait()
	dur = time.Since(t0)
	fmt.Printf("%d items swapin took %v -> %v items/s\n", total, dur, float64(total)/float64(dur.Seconds()))
}

func TestPlasmaSwapper(t *testing.T) {
	os.RemoveAll("teststore.data")
	s := newTestIntPlasmaStore(testCfg)
	defer s.Close()

	n := 1000000
	w := s.NewWriter()
	for i := 0; i < n; i++ {
		w.Insert(skiplist.NewIntKeyItem(i))
	}

	s.EvictAll()
	for i := 0; i < n; i++ {
		itm := skiplist.NewIntKeyItem(i)
		got, _ := w.Lookup(itm)
		if skiplist.CompareInt(itm, got) != 0 {
			t.Errorf("mismatch %d != %d", i, skiplist.IntFromItem(got))
		}
	}

	sts := s.GetStats()
	if sts.NumRecordSwapOut == 0 {
		t.Errorf("Expected few pages to be swapped out")
	}

	/*
		FIXME: Test requires update
		if sts.NumRecordSwapOut != sts.NumPagesSwapIn {
			t.Errorf("Pages swapped out != swapped in (%d != %d)", sts.NumPagesSwapOut, sts.NumPagesSwapIn)
		}
	*/

	fmt.Println(sts)
}

func TestPlasmaAutoSwapper(t *testing.T) {
	defer SetMemoryQuota(maxMemoryQuota)
	var wg sync.WaitGroup

	os.RemoveAll("teststore.data")
	numThreads := 8
	n := 10000000
	nPerThr := n / numThreads
	cfg := testCfg
	cfg.LSSCleanerThreshold = 10
	cfg.AutoLSSCleaning = true
	cfg.AutoSwapper = true
	SetMemoryQuota(1024 * 1024 * 1024)

	s := newTestIntPlasmaStore(cfg)
	defer s.Close()
	total := numThreads * nPerThr

	t0 := time.Now()
	for i := 0; i < numThreads; i++ {
		wg.Add(1)
		w := s.NewWriter()
		go doInsert(w, &wg, i, nPerThr)
	}
	wg.Wait()

	dur := time.Since(t0)
	fmt.Printf("%d items insert took %v -> %v items/s\n", total, dur, float64(total)/float64(dur.Seconds()))

	t0 = time.Now()
	for i := 0; i < numThreads; i++ {
		wg.Add(1)
		w := s.NewWriter()
		go doUpdate(w, &wg, i, nPerThr)
	}
	wg.Wait()
	dur = time.Since(t0)
	fmt.Printf("%d items update took %v -> %v items/s\n", total, dur, float64(total)/float64(dur.Seconds()))

	fmt.Println(s.GetStats())

	frag, ds, used := s.GetLSSInfo()

	fmt.Printf("LSSInfo: frag:%d, ds:%d, used:%d\n", frag, ds, used)

}

// Robert Jenkins 32 bit integer
func intHash(x int) int {
	a := uint32(x)
	a = (a + 0x7ed55d16) + (a << 12)
	a = (a ^ 0xc761c23c) ^ (a >> 19)
	a = (a + 0x165667b1) + (a << 5)
	a = (a + 0xd3a2646c) ^ (a << 9)
	a = (a + 0xfd7046c5) + (a << 3)
	a = (a ^ 0xb55a4f09) ^ (a >> 16)
	return int(a)
}

func TestConcurrDelOps(t *testing.T) {
	var wg sync.WaitGroup

	os.RemoveAll("teststore.data")
	numThreads := 6
	n := 10000000
	nPerThr := n / numThreads
	testCfg.MinPageItems = 200
	s := newTestIntPlasmaStore(testCfg)
	defer s.Close()

	worker := func(w *Writer, wg *sync.WaitGroup, id int, insert bool, n int) {
		defer wg.Done()

		for i := 0; i < n; i++ {
			val := intHash(i + id*n)
			itm := skiplist.NewIntKeyItem(val)
			if insert {
				w.Insert(itm)
			} else {
				w.Delete(itm)
			}
		}
	}

	for i := 0; i < numThreads; i++ {
		wg.Add(1)
		w := s.NewWriter()
		go worker(w, &wg, i, true, nPerThr)
	}
	wg.Wait()

	for i := 0; i < numThreads; i++ {
		wg.Add(1)
		w := s.NewWriter()
		go worker(w, &wg, i, false, nPerThr)
	}
	wg.Wait()

	c := 0
	itr := s.NewIterator()
	for itr.SeekFirst(); itr.Valid(); itr.Next() {
		c++
	}

	if c != 0 {
		t.Errorf("Expected 0, got %d", c)
	}

	fmt.Println(s.GetStats())
}

func TestIteratorSetEnd(t *testing.T) {
	os.RemoveAll("teststore.data")
	s := newTestIntPlasmaStore(testCfg)
	defer s.Close()
	w := s.NewWriter()
	for i := 0; i < 100000; i++ {
		w.Insert(skiplist.NewIntKeyItem(i))
	}

	i := 0
	itr := s.NewIterator().(*Iterator)
	itr.SetEndKey(skiplist.NewIntKeyItem(10000))
	for itr.SeekFirst(); itr.Valid(); itr.Next() {
		if v := skiplist.IntFromItem(itr.Get()); v != i {
			t.Errorf("expected %d, got %d", i, v)
		}

		i++
	}

	if i != 10000 {
		t.Errorf("expected %d, got %d", 10000, i)
	}

}
