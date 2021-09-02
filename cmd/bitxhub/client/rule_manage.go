package client

import (
	"encoding/json"
	"fmt"
	"strconv"

	"github.com/fatih/color"
	ruleMgr "github.com/meshplus/bitxhub-core/rule-mgr"
	"github.com/meshplus/bitxhub-model/constant"
	"github.com/meshplus/bitxhub-model/pb"
	"github.com/tidwall/gjson"
	"github.com/urfave/cli"
)

func ruleMgrCMD() cli.Command {
	return cli.Command{
		Name:  "rule",
		Usage: "rule manage command",
		Subcommands: cli.Commands{
			cli.Command{
				Name:  "all",
				Usage: "query all rules info of one chain",
				Flags: []cli.Flag{
					cli.StringFlag{
						Name:     "id",
						Usage:    "Specify chain id",
						Required: true,
					},
				},
				Action: getRulesList,
			},
			cli.Command{
				Name:  "master",
				Usage: "query master rule address of a chain",
				Flags: []cli.Flag{
					cli.StringFlag{
						Name:     "id",
						Usage:    "Specify chain id",
						Required: true,
					},
				},
				Action: getMasterRuleAddress,
			},
			cli.Command{
				Name:  "status",
				Usage: "query rule status by rule address and chain id",
				Flags: []cli.Flag{
					cli.StringFlag{
						Name:     "id",
						Usage:    "Specify chain id",
						Required: true,
					},
					cli.StringFlag{
						Name:     "addr",
						Usage:    "Specify rule addr",
						Required: true,
					},
				},
				Action: getRuleStatus,
			},
		},
	}
}

func getRulesList(ctx *cli.Context) error {
	id := ctx.String("id")

	receipt, err := invokeBVMContractBySendView(ctx, constant.RuleManagerContractAddr.String(), "Rules", pb.String(id))
	if err != nil {
		return err
	}

	if receipt.IsSuccess() {
		rules := make([]*ruleMgr.Rule, 0)
		if receipt.Ret != nil {
			if err := json.Unmarshal(receipt.Ret, &rules); err != nil {
				return fmt.Errorf("unmarshal rules error: %w", err)
			}
		}
		printRule(rules)
	} else {
		color.Red("get rules error: %s\n", string(receipt.Ret))
	}
	return nil
}

func getMasterRuleAddress(ctx *cli.Context) error {
	id := ctx.String("id")

	receipt, err := invokeBVMContractBySendView(ctx, constant.RuleManagerContractAddr.String(), "GetMasterRule", pb.String(id))
	if err != nil {
		return err
	}
	rule := &ruleMgr.Rule{}
	if err := json.Unmarshal(receipt.Ret, rule); err != nil {
		return err
	}

	if receipt.IsSuccess() {
		color.Green("available rule address is %s", rule.Address)
	} else {
		color.Red("get available rule address error: %s\n", string(receipt.Ret))
	}
	return nil
}

func getRuleStatus(ctx *cli.Context) error {
	chainId := ctx.String("id")
	ruleAddr := ctx.String("addr")

	receipt, err := invokeBVMContractBySendView(ctx, constant.RuleManagerContractAddr.String(), "GetRuleByAddr", pb.String(chainId), pb.String(ruleAddr))
	if err != nil {
		return err
	}

	if receipt.IsSuccess() {
		rule := &ruleMgr.Rule{}
		if err := json.Unmarshal(receipt.Ret, rule); err != nil {
			return fmt.Errorf("unmarshal receipt error: %w", err)
		}
		color.Green("the rule %s is %s", ruleAddr, string(rule.Status))
	} else {
		color.Red("get rule status error: %s\n", string(receipt.Ret))
	}
	return nil
}

func updateRule(ctx *cli.Context) error {
	id := ctx.String("id")
	addr := ctx.String("addr")
	reason := ctx.String("reason")

	receipt, err := invokeBVMContract(ctx, constant.RuleManagerContractAddr.String(), "UpdateMasterRule", pb.String(id), pb.String(addr), pb.String(reason))
	if err != nil {
		return err
	}

	if receipt.IsSuccess() {
		proposalId := gjson.Get(string(receipt.Ret), "proposal_id").String()
		color.Green("proposal id is %s", proposalId)
	} else {
		color.Red("update rule error: %s\n", string(receipt.Ret))
	}
	return nil
}

func printRule(rules []*ruleMgr.Rule) {
	var table [][]string
	table = append(table, []string{"ChainID", "RuleAddress", "Status", "Master"})

	for _, r := range rules {
		table = append(table, []string{
			r.ChainID,
			r.Address,
			string(r.Status),
			strconv.FormatBool(r.Master),
		})
	}

	PrintTable(table, true)
}
