/*
 * Copyright (C) 2018 The ZeepinChain Authors
 * This file is part of The ZeepinChain library.
 *
 * The ZeepinChain is free software: you can redistribute it and/or modify
 * it under the terms of the GNU Lesser General Public License as published by
 * the Free Software Foundation, either version 3 of the License, or
 * (at your option) any later version.
 *
 * The ZeepinChain is distributed in the hope that it will be useful,
 * but WITHOUT ANY WARRANTY; without even the implied warranty of
 * MERCHANTABILITY or FITNESS FOR A PARTICULAR PURPOSE.  See the
 * GNU Lesser General Public License for more details.
 *
 * You should have received a copy of the GNU Lesser General Public License
 * along with The ZeepinChain.  If not, see <http://www.gnu.org/licenses/>.
 */

package handlers

import (
	"bytes"
	"encoding/hex"
	"encoding/json"
	"fmt"
	clisvrcom "github.com/imZhuFei/zeepin/cmd/sigsvr/common"
	cliutil "github.com/imZhuFei/zeepin/cmd/utils"
	"github.com/imZhuFei/zeepin/common"
	"github.com/imZhuFei/zeepin/common/log"
	httpcom "github.com/imZhuFei/zeepin/http/base/common"
)

type SigNeoVMInvokeTxReq struct {
	GasPrice uint64        `json:"gas_price"`
	GasLimit uint64        `json:"gas_limit"`
	Address  string        `json:"address"`
	Params   []interface{} `json:"params"`
}

type SigNeoVMInvokeTxRsp struct {
	SignedTx string `json:"signed_tx"`
}

func SigNeoVMInvokeTx(req *clisvrcom.CliRpcRequest, resp *clisvrcom.CliRpcResponse) {
	rawReq := &SigNeoVMInvokeTxReq{}
	err := json.Unmarshal(req.Params, rawReq)
	if err != nil {
		log.Infof("SigNeoVMInvokeTx json.Unmarshal SigNeoVMInvokeTxReq:%s error:%s", req.Params, err)
		resp.ErrorCode = clisvrcom.CLIERR_INVALID_PARAMS
		return
	}
	params, err := cliutil.ParseNeoVMInvokeParams(rawReq.Params)
	if err != nil {
		resp.ErrorCode = clisvrcom.CLIERR_INVALID_PARAMS
		resp.ErrorInfo = fmt.Sprintf("ParseNeoVMInvokeParams error:%s", err)
		return
	}
	contAddr, err := common.AddressFromHexString(rawReq.Address)
	if err != nil {
		log.Infof("Cli Qid:%s SigNeoVMInvokeTx AddressParseFromBytes:%s error:%s", req.Qid, rawReq.Address, err)
		resp.ErrorCode = clisvrcom.CLIERR_INVALID_PARAMS
		return
	}
	tx, err := httpcom.NewNeovmInvokeTransaction(rawReq.GasPrice, rawReq.GasLimit, contAddr, params)
	if err != nil {
		log.Infof("Cli Qid:%s SigNeoVMInvokeTx InvokeNeoVMContractTx error:%s", req.Qid, err)
		resp.ErrorCode = clisvrcom.CLIERR_INVALID_PARAMS
		return
	}
	signer := clisvrcom.DefAccount
	err = cliutil.SignTransaction(signer, tx)
	if err != nil {
		log.Infof("Cli Qid:%s SigNeoVMInvokeTx SignTransaction error:%s", req.Qid, err)
		resp.ErrorCode = clisvrcom.CLIERR_INTERNAL_ERR
		return
	}
	buf := bytes.NewBuffer(nil)
	err = tx.Serialize(buf)
	if err != nil {
		log.Infof("Cli Qid:%s SigNeoVMInvokeTx tx Serialize error:%s", req.Qid, err)
		resp.ErrorCode = clisvrcom.CLIERR_INTERNAL_ERR
		return
	}
	resp.Result = &SigNeoVMInvokeTxRsp{
		SignedTx: hex.EncodeToString(buf.Bytes()),
	}
}