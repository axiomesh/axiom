package repo

import (
	"encoding/json"
	"fmt"
	"io/ioutil"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/ethereum/go-ethereum/event"
	"github.com/fsnotify/fsnotify"
	"github.com/mitchellh/go-homedir"
	ma "github.com/multiformats/go-multiaddr"
	"github.com/spf13/viper"
)

const (
	// defaultPathName is the default config dir name
	defaultPathName = ".axiom"
	// defaultPathRoot is the path to the default config dir location.
	defaultPathRoot = "~/" + defaultPathName
	// envDir is the environment variable used to change the path root.
	envDir = "AXIOM_PATH"
	// Config name
	configName = "axiom.toml"
	// key name
	KeyName = "key.json"
	// API name
	APIName = "api"
	// admin weight
	SuperAdminWeight  = 2
	NormalAdminWeight = 1
	// governance strategy default participate threshold
	DefaultSimpleMajorityExpression = "a > 0.5 * t"
	DefaultZeroStrategyExpression   = "a >= 0"
	//Passwd
	DefaultPasswd = "bitxhub"

	SuperMajorityApprove = "SuperMajorityApprove"
	SuperMajorityAgainst = "SuperMajorityAgainst"
	SimpleMajority       = "SimpleMajority"
	ZeroPermission       = "ZeroPermission"

	AppchainMgr         = "appchain_mgr"
	RuleMgr             = "rule_mgr"
	NodeMgr             = "node_mgr"
	ServiceMgr          = "service_mgr"
	RoleMgr             = "role_mgr"
	ProposalStrategyMgr = "proposal_strategy_mgr"
	DappMgr             = "dapp_mgr"
	AllMgr              = "all_mgr"
)

type Config struct {
	RepoRoot      string        `json:"repo_root"`
	Title         string        `json:"title"`
	Solo          bool          `json:"solo"`
	RPCGasCap     uint64        `json:"rpc_gas_cap"`
	RPCEVMTimeout time.Duration `json:"rpc_evm_timeout"`
	Port          Port          `json:"port"`
	PProf         PProf         `json:"pprof"`
	Monitor       Monitor       `json:"monitor"`
	JLimiter      JLimiter      `json:"jlimiter"`
	P2pLimit      P2pLimiter    `toml:"p2p_limiter" json:"p2p_limiter"`
	Ping          Ping          `json:"ping"`
	Log           Log           `json:"log"`
	Txpool        Txpool        `json:"txpool"`
	Order         Order         `json:"order"`
	Executor      Executor      `json:"executor"`
	Ledger        Ledger        `json:"ledger"`
	Genesis       Genesis       `json:"genesis"`
	Security      Security      `toml:"security" json:"security"`
	Crypto        Crypto        `toml:"crypto" json:"crypto"`
}

type Monitor struct {
	Enable bool
}

// Security are files used to setup connection with tls
type Security struct {
	EnableTLS   bool   `mapstructure:"enable_tls"`
	PemFilePath string `mapstructure:"pem_file_path" json:"pem_file_path"`
}

type Port struct {
	JsonRpc   int64 `toml:"jsonrpc" json:"jsonrpc"`
	Grpc      int64 `toml:"grpc" json:"grpc"`
	Gateway   int64 `toml:"gateway" json:"gateway"`
	PProf     int64 `toml:"pprof" json:"pprof"`
	Monitor   int64 `toml:"monitor" json:"monitor"`
	WebSocket int64 `toml:"websocket" json:"websocket"`
}

type PProf struct {
	Enable   bool          `toml:"enbale" json:"enable"`
	PType    string        `toml:"ptype" json:"ptype"`
	Mode     string        `toml:"mode" json:"mode"`
	Duration time.Duration `toml:"duration" json:"duration"`
}

type JLimiter struct {
	Interval time.Duration `toml:"interval" json:"interval"`
	Quantum  int64         `toml:"quantum" json:"quantum"`
	Capacity int64         `toml:"capacity" json:"capacity"`
}

type P2pLimiter struct {
	Limit int64 `toml:"limit" json:"limit"`
	Burst int64 `toml:"burst" json:"burst"`
}

type Ping struct {
	Enable   bool          `toml:"enable" json:"enable"`
	Duration time.Duration `toml:"duration" json:"duration"`
}

type Log struct {
	Level        string    `toml:"level" json:"level"`
	Dir          string    `toml:"dir" json:"dir"`
	Filename     string    `toml:"filename" json:"filename"`
	ReportCaller bool      `mapstructure:"report_caller" json:"report_caller"`
	Module       LogModule `toml:"module" json:"module"`
}

type LogModule struct {
	P2P       string `toml:"p2p" json:"p2p"`
	Consensus string `toml:"consensus" json:"consensus"`
	Executor  string `toml:"executor" json:"executor"`
	Router    string `toml:"router" json:"router"`
	API       string `toml:"api" json:"api"`
	CoreAPI   string `mapstructure:"coreapi" toml:"coreapi" json:"coreapi"`
	Storage   string `toml:"storage" json:"storage"`
	Profile   string `toml:"profile" json:"profile"`
	TSS       string `toml:"tss" json:"tss"`
	Finance   string `toml:"finance" json:"finance"`
}

type Genesis struct {
	ChainID       uint64   `json:"chainid" toml:"chainid"`
	GasLimit      uint64   `mapstructure:"gas_limit" json:"gas_limit" toml:"gas_limit"`
	GasPrice      uint64   `mapstructure:"gas_price" json:"gas_price"`
	MaxGasPrice   uint64   `mapstructure:"max_gas_price" json:"max_gas_price"`
	MinGasPrice   uint64   `mapstructure:"min_gas_price" json:"min_gas_price"`
	GasChangeRate float64  `mapstructure:"gas_change_rate" json:"gas_change_rate"`
	Balance       string   `json:"balance" toml:"balance"`
	Admins        []*Admin `json:"admins" toml:"admins"`
}

type Admin struct {
	Address string `json:"address" toml:"address"`
	Weight  uint64 `json:"weight" toml:"weight"`
}

type Txpool struct {
	BatchSize    int           `mapstructure:"batch_size" json:"batch_size"`
	BatchTimeout time.Duration `mapstructure:"batch_timeout" json:"batch_timeout"`
}

type Order struct {
	Type string `toml:"type" json:"type"`
}

type Executor struct {
	Type string `toml:"type" json:"type"`
}

type Ledger struct {
	Type string `toml:"type" json:"type"`
	Kv   string `toml:"kv" json:"kv"`
}

type Crypto struct {
	Algorithms []string `json:"algorithms" toml:"algorithms"`
}

func (c *Config) Bytes() ([]byte, error) {
	ret, err := json.Marshal(c)
	if err != nil {
		return nil, err
	}

	return ret, nil
}

func DefaultConfig() (*Config, error) {
	return &Config{
		Title: "Axiom configuration file",
		Solo:  false,
		Port: Port{
			Grpc:      60011,
			Gateway:   9091,
			PProf:     53121,
			Monitor:   40011,
			WebSocket: 9092,
		},
		RPCGasCap:     300000000,
		RPCEVMTimeout: 5 * time.Second,
		PProf:         PProf{Enable: false},
		Ping:          Ping{Enable: false},
		Log: Log{
			Level:    "info",
			Dir:      "logs",
			Filename: "axiom.log",
			Module: LogModule{
				P2P:       "info",
				Consensus: "debug",
				Executor:  "info",
				Router:    "info",
				API:       "info",
				CoreAPI:   "info",
				TSS:       "info",
			},
		},
		Txpool: Txpool{
			BatchSize:    500,
			BatchTimeout: 500 * time.Millisecond,
		},
		Order: Order{
			Type: "rbft",
		},
		Executor: Executor{
			Type: "serial",
		},
		Genesis: Genesis{
			ChainID:       1,
			GasLimit:      0x5f5e100,
			GasChangeRate: 0.125,
			MaxGasPrice:   10000,
			MinGasPrice:   1000,
			GasPrice:      5000,
			Balance:       "1000000000000000000",
		},
		Ledger: Ledger{Type: "complex"},
		Crypto: Crypto{Algorithms: []string{"Secp256k1"}},
		JLimiter: JLimiter{
			Interval: 50,
			Quantum:  500,
			Capacity: 10000,
		},
		P2pLimit: P2pLimiter{
			Limit: 10000,
			Burst: 10000,
		},
	}, nil
}

func UnmarshalConfig(viper *viper.Viper, repoRoot string, configPath string) (*Config, error) {
	if len(configPath) == 0 {
		viper.SetConfigFile(filepath.Join(repoRoot, configName))
	} else {
		viper.SetConfigFile(configPath)
		fileData, err := ioutil.ReadFile(configPath)
		if err != nil {
			return nil, fmt.Errorf("read axiom config error: %w", err)
		}
		err = ioutil.WriteFile(filepath.Join(repoRoot, configName), fileData, 0644)
		if err != nil {
			return nil, fmt.Errorf("write axiom config failed: %w", err)
		}
	}
	viper.SetConfigType("toml")
	viper.AutomaticEnv()
	viper.SetEnvPrefix("AXIOM")
	replacer := strings.NewReplacer(".", "_")
	viper.SetEnvKeyReplacer(replacer)
	if err := viper.ReadInConfig(); err != nil {
		return nil, fmt.Errorf("readInConfig error: %w", err)
	}

	config, err := DefaultConfig()
	if err != nil {
		return nil, err
	}

	if err := viper.Unmarshal(config); err != nil {
		return nil, fmt.Errorf("unmarshal config error: %w", err)
	}

	config.RepoRoot = repoRoot
	return config, nil
}

func WatchAxiomConfig(viper *viper.Viper, feed *event.Feed) {
	viper.WatchConfig()
	viper.OnConfigChange(func(in fsnotify.Event) {
		fmt.Println("axiom config file changed: ", in.String())

		config, err := DefaultConfig()
		if err != nil {
			fmt.Println("get default config: ", err)
			return
		}

		if err := viper.Unmarshal(config); err != nil {
			fmt.Println("unmarshal config: ", err)
			return
		}

		feed.Send(&Repo{Config: config})
	})
}

func WatchNetworkConfig(viper *viper.Viper, feed *event.Feed) {
	viper.WatchConfig()
	viper.OnConfigChange(func(in fsnotify.Event) {
		fmt.Println("network config file changed: ", in.String())
		var config *NetworkConfig
		if err := viper.Unmarshal(config); err != nil {
			fmt.Println("unmarshal config: ", err)
			return
		}

		checkReaptAddr := make(map[string]uint64)
		for _, node := range config.Nodes {
			if node.ID == config.ID {
				if len(node.Hosts) == 0 {
					fmt.Printf("no hosts found by node:%d \n", node.ID)
					return
				}
				config.LocalAddr = node.Hosts[0]
				addr, err := ma.NewMultiaddr(fmt.Sprintf("%s%s", node.Hosts[0], node.Pid))
				if err != nil {
					fmt.Printf("new multiaddr: %v \n", err)
					return
				}
				config.LocalAddr = strings.Replace(config.LocalAddr, ma.Split(addr)[0].String(), "/ip4/0.0.0.0", -1)
			}

			if _, ok := checkReaptAddr[node.Hosts[0]]; !ok {
				checkReaptAddr[node.Hosts[0]] = node.ID
			} else {
				err := fmt.Errorf("reapt address with Node: nodeID = %d,Host = %s",
					checkReaptAddr[node.Hosts[0]], node.Hosts[0])
				panic(err)
			}
		}

		if config.LocalAddr == "" {
			fmt.Printf("lack of local address \n")
			return
		}

		idx := strings.LastIndex(config.LocalAddr, "/p2p/")
		if idx == -1 {
			fmt.Printf("pid is not existed in bootstrap \n")
			return
		}

		config.LocalAddr = config.LocalAddr[:idx]

		feed.Send(&Repo{NetworkConfig: config})
	})
}

func ReadConfig(v *viper.Viper, path, configType string, config interface{}) error {
	v.SetConfigFile(path)
	v.SetConfigType(configType)
	if err := v.ReadInConfig(); err != nil {
		return fmt.Errorf("readInConfig error: %w", err)
	}

	if err := v.Unmarshal(config); err != nil {
		return fmt.Errorf("unmarshal config error: %w", err)
	}

	return nil
}

func PathRoot() (string, error) {
	dir := os.Getenv(envDir)
	var err error
	if len(dir) == 0 {
		dir, err = homedir.Expand(defaultPathRoot)
	}
	return dir, err
}

func PathRootWithDefault(path string) (string, error) {
	if len(path) == 0 {
		return PathRoot()
	}

	return path, nil
}
