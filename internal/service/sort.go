package service

import "sort"

// sortDiffEntries 按制品名稳定排序差异条目，确保跨运行输出确定。
func sortDiffEntries(entries []DiffEntry) {
	sort.SliceStable(entries, func(i, j int) bool {
		return entries[i].Artifact < entries[j].Artifact
	})
}

// sortPinEntries 按制品名稳定排序锁定 pin 条目，保证锁定快照内容的确定性输出。
func sortPinEntries(entries []LockfilePinEntry) {
	sort.SliceStable(entries, func(i, j int) bool {
		return entries[i].Artifact < entries[j].Artifact
	})
}

// sortResolvedEntries 按制品名稳定排序解析条目，供就绪检查与锁定校验输出稳定。
func sortResolvedEntries(entries []ResolvedEntry) {
	sort.SliceStable(entries, func(i, j int) bool {
		return entries[i].Artifact < entries[j].Artifact
	})
}
