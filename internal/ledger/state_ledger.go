package ledger

import (
	"encoding/json"
	"errors"
	"fmt"
	"math/big"
	"path"
	"time"

	"github.com/ethereum/go-ethereum/common"
	"github.com/sirupsen/logrus"

	"github.com/axiomesh/axiom-bft/common/consensus"
	"github.com/axiomesh/axiom-kit/jmt"
	"github.com/axiomesh/axiom-kit/storage/kv"
	"github.com/axiomesh/axiom-kit/types"
	"github.com/axiomesh/axiom-ledger/internal/ledger/prune"
	"github.com/axiomesh/axiom-ledger/internal/ledger/snapshot"
	"github.com/axiomesh/axiom-ledger/internal/ledger/utils"
	"github.com/axiomesh/axiom-ledger/internal/storagemgr"
	"github.com/axiomesh/axiom-ledger/pkg/loggers"
	"github.com/axiomesh/axiom-ledger/pkg/repo"
)

var _ StateLedger = (*StateLedgerImpl)(nil)

var (
	ErrorRollbackToHigherNumber = errors.New("rollback to higher blockchain height")
)

// maxBatchSize defines the maximum size of the data in single batch write operation, which is 64 MB.
const maxBatchSize = 64 * 1024 * 1024

type revision struct {
	id           int
	changerIndex int
}

type StateLedgerImpl struct {
	logger       logrus.FieldLogger
	accountCache *AccountCache // todo remove this
	accountTrie  *jmt.JMT      // keep track of the latest world state (dirty or committed)

	pruneCache       *prune.PruneCache
	backend          kv.Storage
	accountTrieCache *storagemgr.CacheWrapper
	storageTrieCache *storagemgr.CacheWrapper

	triePreloader *triePreloader
	accounts      map[string]IAccount
	repo          *repo.Repo
	blockHeight   uint64
	thash         *types.Hash
	txIndex       int

	validRevisions []revision
	nextRevisionId int
	changer        *stateChanger

	accessList *AccessList
	preimages  map[types.Hash][]byte
	refund     uint64
	logs       *evmLogs

	snapshot *snapshot.Snapshot

	transientStorage transientStorage

	// enableExpensiveMetric determines if costly metrics gathering is allowed or not.
	// The goal is to separate standard metrics for health monitoring and debug metrics that might impact runtime performance.
	enableExpensiveMetric bool
}

type SnapshotMeta struct {
	BlockHeader *types.BlockHeader
	EpochInfo   *types.EpochInfo
	Nodes       *consensus.QuorumValidators
}

type snapshotMetaMarshalHelper struct {
	BlockHeader []byte `json:"block_header"`
	EpochInfo   []byte `json:"epoch_info"`
	Nodes       []byte `json:"nodes"`
}

func (m *SnapshotMeta) Marshal() ([]byte, error) {
	blockHeader, err := m.BlockHeader.Marshal()
	if err != nil {
		return nil, err
	}
	epochInfo, err := m.EpochInfo.Marshal()
	if err != nil {
		return nil, err
	}
	nodes, err := m.Nodes.MarshalVTStrict()
	if err != nil {
		return nil, err
	}

	return json.Marshal(&snapshotMetaMarshalHelper{
		BlockHeader: blockHeader,
		EpochInfo:   epochInfo,
		Nodes:       nodes,
	})
}

func (m *SnapshotMeta) Unmarshal(data []byte) error {
	var helper snapshotMetaMarshalHelper
	if err := json.Unmarshal(data, &helper); err != nil {
		return err
	}

	blockHeader := &types.BlockHeader{}
	err := blockHeader.Unmarshal(helper.BlockHeader)
	if err != nil {
		return err
	}
	epochInfo := &types.EpochInfo{}
	err = epochInfo.Unmarshal(helper.EpochInfo)
	if err != nil {
		return err
	}

	nodes := &consensus.QuorumValidators{}
	err = nodes.UnmarshalVT(helper.Nodes)
	if err != nil {
		return err
	}

	m.BlockHeader = blockHeader
	m.EpochInfo = epochInfo
	m.Nodes = nodes

	return nil
}

// NewView get a view at specific block. We can enable snapshot if and only if the block were the latest block.
func (l *StateLedgerImpl) NewView(blockHeader *types.BlockHeader, enableSnapshot bool) (StateLedger, error) {
	l.logger.Debugf("[NewView] height: %v, stateRoot: %v", blockHeader.Number, blockHeader.StateRoot)
	if l.repo.Config.Ledger.EnablePrune {
		min, max := l.GetHistoryRange()
		if blockHeader.Number < min || blockHeader.Number > max {
			return nil, fmt.Errorf("history at target block %v is invalid, the valid range is from %v to %v", blockHeader.Number, min, max)
		}
	}

	lg := &StateLedgerImpl{
		repo:                  l.repo,
		logger:                l.logger,
		backend:               l.backend,
		pruneCache:            l.pruneCache,
		accountTrieCache:      l.accountTrieCache,
		storageTrieCache:      l.storageTrieCache,
		accountCache:          l.accountCache,
		accounts:              make(map[string]IAccount),
		preimages:             make(map[types.Hash][]byte),
		changer:               newChanger(),
		accessList:            NewAccessList(),
		logs:                  newEvmLogs(),
		enableExpensiveMetric: l.enableExpensiveMetric,
		blockHeight:           blockHeader.Number,
	}
	if enableSnapshot {
		lg.snapshot = l.snapshot
	}
	lg.refreshAccountTrie(blockHeader.StateRoot)
	return lg, nil
}

// NewViewWithoutCache get a view ledger at specific block. We can enable snapshot if and only if the block were the latest block.
func (l *StateLedgerImpl) NewViewWithoutCache(blockHeader *types.BlockHeader, enableSnapshot bool) (StateLedger, error) {
	l.logger.Debugf("[NewViewWithoutCache] height: %v, stateRoot: %v", blockHeader.Number, blockHeader.StateRoot)
	if l.repo.Config.Ledger.EnablePrune {
		min, max := l.GetHistoryRange()
		if blockHeader.Number < min || blockHeader.Number > max {
			return nil, fmt.Errorf("history at target block %v is invalid, the valid range is from %v to %v", blockHeader.Number, min, max)
		}
	}

	ac, _ := NewAccountCache(0, true)
	lg := &StateLedgerImpl{
		repo:                  l.repo,
		logger:                l.logger,
		backend:               l.backend,
		pruneCache:            l.pruneCache,
		accountTrieCache:      l.accountTrieCache,
		storageTrieCache:      l.storageTrieCache,
		accountCache:          ac,
		accounts:              make(map[string]IAccount),
		preimages:             make(map[types.Hash][]byte),
		changer:               newChanger(),
		accessList:            NewAccessList(),
		logs:                  newEvmLogs(),
		enableExpensiveMetric: l.enableExpensiveMetric,
		blockHeight:           blockHeader.Number,
	}
	if enableSnapshot {
		lg.snapshot = l.snapshot
	}
	lg.refreshAccountTrie(blockHeader.StateRoot)
	return lg, nil
}

func (l *StateLedgerImpl) GetHistoryRange() (uint64, uint64) {
	minHeight := uint64(0)
	maxHeight := uint64(0)

	data := l.backend.Get(utils.CompositeKey(utils.PruneJournalKey, utils.MinHeightStr))
	if data != nil {
		minHeight = utils.UnmarshalHeight(data)
	}

	data = l.backend.Get(utils.CompositeKey(utils.PruneJournalKey, utils.MaxHeightStr))
	if data != nil {
		maxHeight = utils.UnmarshalHeight(data)
	}

	return minHeight, maxHeight
}

func (l *StateLedgerImpl) GetStateDelta(blockNumber uint64) *types.StateDelta {
	return l.pruneCache.GetStateDelta(blockNumber)
}

func (l *StateLedgerImpl) Finalise() {
	for _, account := range l.accounts {
		keys := account.Finalise()

		if l.triePreloader != nil {
			l.triePreloader.preload(common.Hash{}, [][]byte{utils.CompositeAccountKey(account.GetAddress())})
			if len(keys) > 0 {
				l.triePreloader.preload(account.GetStorageRootHash(), keys)
			}
		}
		account.SetCreated(false)
	}

	l.ClearChangerAndRefund()
}

func (l *StateLedgerImpl) IterateTrie(snapshotMeta *SnapshotMeta, kv kv.Storage, errC chan error) {
	stateRoot := snapshotMeta.BlockHeader.StateRoot.ETHHash()
	l.logger.Infof("[IterateTrie] blockhash: %v, rootHash: %v", snapshotMeta.BlockHeader.Hash(), stateRoot)
	batch := kv.NewBatch()

	if l.pruneCache != nil {
		if err := l.pruneCache.Rollback(blockHeader.Number, false); err != nil {
			errC <- err
		}
		batch.Put(utils.CompositeKey(utils.PruneJournalKey, utils.MinHeightStr), utils.MarshalHeight(blockHeader.Number))
		batch.Put(utils.CompositeKey(utils.PruneJournalKey, utils.MaxHeightStr), utils.MarshalHeight(blockHeader.Number))
	}

	queue := []common.Hash{stateRoot}
	for len(queue) > 0 {
		trieRoot := queue[0]
		iter := jmt.NewIterator(trieRoot, l.backend, l.pruneCache, 10000, 300*time.Second)
		l.logger.Debugf("[IterateTrie] trie root=%v", trieRoot)
		go iter.Iterate()

		for {
			node, err := iter.Next()
			if err != nil {
				if err == jmt.ErrorNoMoreData {
					break
				} else {
					errC <- err
					return
				}
			}
			batch.Put(node.RawKey, node.RawValue)
			// data size exceed threshold, flush to disk
			if batch.Size() > maxBatchSize {
				batch.Commit()
				batch.Reset()
				l.logger.Infof("[IterateTrie] write batch periodically")
			}
			if trieRoot == stateRoot && len(node.LeafValue) > 0 {
				// resolve potential contract account
				acc := &types.InnerAccount{Balance: big.NewInt(0)}
				if err := acc.Unmarshal(node.LeafValue); err != nil {
					panic(err)
				}
				if acc.StorageRoot != (common.Hash{}) {
					// set contract code
					codeKey := utils.CompositeCodeKey(types.NewAddress(types.HexToBytes(node.LeafKey)), acc.CodeHash)
					batch.Put(codeKey, l.backend.Get(codeKey))
					// prepare storage trie root
					queue = append(queue, acc.StorageRoot)
				}
			}
		}
		queue = queue[1:]
		l.logger.Infof("[IterateTrie] trieRoot=%v, rootNodeKey from kv=%v", trieRoot, l.backend.Get(trieRoot[:]))
		batch.Put(trieRoot[:], l.backend.Get(trieRoot[:]))
	}

	snapshotMetaBytes, err := snapshotMeta.Marshal()
	if err != nil {
		errC <- err
		return
	}
	batch.Put([]byte(utils.SnapshotMetaKey), snapshotMetaBytes)

	batch.Commit()
	l.logger.Infof("[IterateTrie] iterate trie successfully")

	errC <- nil
}

func (l *StateLedgerImpl) GetTrieSnapshotMeta() (*SnapshotMeta, error) {
	raw := l.backend.Get([]byte(utils.SnapshotMetaKey))
	if len(raw) == 0 {
		return nil, ErrNotFound
	}

	snapshotMeta := &SnapshotMeta{}
	if err := snapshotMeta.Unmarshal(raw); err != nil {
		return nil, err
	}
	return snapshotMeta, nil
}

func (l *StateLedgerImpl) GenerateSnapshot(blockHeader *types.BlockHeader, errC chan error) {
	stateRoot := blockHeader.StateRoot.ETHHash()
	l.logger.Infof("[GenerateSnapshot] blockNum: %v, blockhash: %v, rootHash: %v", blockHeader.Number, blockHeader.Hash(), stateRoot)

	queue := []common.Hash{stateRoot}
	batch := l.snapshot.Batch()
	for len(queue) > 0 {
		trieRoot := queue[0]
		iter := jmt.NewIterator(trieRoot, l.backend, l.pruneCache, 10000, 300*time.Second)
		l.logger.Debugf("[GenerateSnapshot] trie root=%v", trieRoot)
		go iter.IterateLeaf()

		for {
			node, err := iter.Next()
			if err != nil {
				if err == jmt.ErrorNoMoreData {
					break
				} else {
					errC <- err
					return
				}
			}
			batch.Put(node.LeafKey, node.LeafValue)
			// data size exceed threshold, flush to disk
			if batch.Size() > maxBatchSize {
				batch.Commit()
				batch.Reset()
				l.logger.Infof("[GenerateSnapshot] write batch periodically")
			}
			if trieRoot == stateRoot && len(node.LeafValue) > 0 {
				// resolve potential contract account
				acc := &types.InnerAccount{Balance: big.NewInt(0)}
				if err := acc.Unmarshal(node.LeafValue); err != nil {
					panic(err)
				}
				if acc.StorageRoot != (common.Hash{}) {
					// prepare storage trie root
					queue = append(queue, acc.StorageRoot)
				}
			}
		}
		queue = queue[1:]
		batch.Put(trieRoot[:], l.backend.Get(trieRoot[:]))
	}
	batch.Commit()
	l.logger.Infof("[GenerateSnapshot] generate snapshot successfully")

	errC <- nil
}

func (l *StateLedgerImpl) VerifyTrie(blockHeader *types.BlockHeader) (bool, error) {
	l.logger.Infof("[VerifyTrie] start verifying blockNumber: %v, rootHash: %v", blockHeader.Number, blockHeader.StateRoot.String())
	defer l.logger.Infof("[VerifyTrie] finish VerifyTrie")
	return jmt.VerifyTrie(blockHeader.StateRoot.ETHHash(), l.backend, l.pruneCache)
}

func (l *StateLedgerImpl) Prove(rootHash common.Hash, key []byte) (*jmt.ProofResult, error) {
	var trie *jmt.JMT
	if rootHash == (common.Hash{}) {
		trie = l.accountTrie
		return trie.Prove(key)
	}
	trie, err := jmt.New(rootHash, l.backend, nil, l.pruneCache, l.logger)
	if err != nil {
		return nil, err
	}
	return trie.Prove(key)
}

func newStateLedger(rep *repo.Repo, stateStorage, snapshotStorage kv.Storage) (StateLedger, error) {
	stateCachedStorage := storagemgr.NewCachedStorage(stateStorage, 128).(*storagemgr.CachedStorage)
	accountTrieCache := storagemgr.NewCacheWrapper(rep.Config.Ledger.StateLedgerAccountTrieCacheMegabytesLimit, true)
	storageTrieCache := storagemgr.NewCacheWrapper(rep.Config.Ledger.StateLedgerStorageTrieCacheMegabytesLimit, true)

	accountCache, err := NewAccountCache(rep.Config.Ledger.StateLedgerAccountCacheSize, false)
	if err != nil {
		return nil, err
	}
	accountCache.SetEnableExpensiveMetric(rep.Config.Monitor.EnableExpensive)

	ledger := &StateLedgerImpl{
		repo:                  rep,
		logger:                loggers.Logger(loggers.Storage),
		backend:               stateCachedStorage,
		accountTrieCache:      accountTrieCache,
		storageTrieCache:      storageTrieCache,
		pruneCache:            prune.NewPruneCache(rep, stateCachedStorage, accountTrieCache, storageTrieCache, loggers.Logger(loggers.Storage)),
		accountCache:          accountCache,
		accounts:              make(map[string]IAccount),
		preimages:             make(map[types.Hash][]byte),
		changer:               newChanger(),
		accessList:            NewAccessList(),
		logs:                  newEvmLogs(),
		enableExpensiveMetric: rep.Config.Monitor.EnableExpensive,
	}

	if snapshotStorage != nil {
		ledger.snapshot = snapshot.NewSnapshot(rep, snapshotStorage, ledger.logger)
	}

	ledger.refreshAccountTrie(nil)

	return ledger, nil
}

// NewStateLedger create a new ledger instance
func NewStateLedger(rep *repo.Repo, storageDir string) (StateLedger, error) {
	stateStoragePath := repo.GetStoragePath(rep.RepoRoot, storagemgr.Ledger)
	if storageDir != "" {
		stateStoragePath = path.Join(storageDir, storagemgr.Ledger)
	}
	stateStorage, err := storagemgr.OpenWithMetrics(stateStoragePath, storagemgr.Ledger)
	if err != nil {
		return nil, fmt.Errorf("create stateDB: %w", err)
	}

	snapshotStoragePath := repo.GetStoragePath(rep.RepoRoot, storagemgr.Snapshot)
	if storageDir != "" {
		snapshotStoragePath = path.Join(storageDir, storagemgr.Snapshot)
	}
	snapshotStorage, err := storagemgr.OpenWithMetrics(snapshotStoragePath, storagemgr.Snapshot)
	if err != nil {
		return nil, fmt.Errorf("create snapshot storage: %w", err)
	}

	return newStateLedger(rep, stateStorage, snapshotStorage)
}

func (l *StateLedgerImpl) SetTxContext(thash *types.Hash, ti int) {
	l.thash = thash
	l.txIndex = ti
}

// Close close the ledger instance
func (l *StateLedgerImpl) Close() {
	_ = l.backend.Close()
	l.triePreloader.close()
}

func (l *StateLedgerImpl) CurrentBlockHeight() uint64 {
	return l.blockHeight
}
