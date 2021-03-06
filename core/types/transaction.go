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

 * Copyright (C) 2018 The ontology Authors
 * This file is part of The ontology library.
 *
 * The ontology is free software: you can redistribute it and/or modify
 * it under the terms of the GNU Lesser General Public License as published by
 * the Free Software Foundation, either version 3 of the License, or
 * (at your option) any later version.
 *
 * The ontology is distributed in the hope that it will be useful,
 * but WITHOUT ANY WARRANTY; without even the implied warranty of
 * MERCHANTABILITY or FITNESS FOR A PARTICULAR PURPOSE.  See the
 * GNU Lesser General Public License for more details.
 *
 * You should have received a copy of the GNU Lesser General Public License
 * along with The ontology.  If not, see <http://www.gnu.org/licenses/>.
 */

package types

import (
	"bytes"
	"crypto/sha256"
	"errors"
	"fmt"
	"io"

	"github.com/imZhuFei/zeepin/common"
	"github.com/imZhuFei/zeepin/common/constants"
	"github.com/imZhuFei/zeepin/common/serialization"
	"github.com/imZhuFei/zeepin/core/payload"
	"github.com/imZhuFei/zeepin/core/program"
	"github.com/ontio/ontology-crypto/keypair"
)

const MAX_TX_SIZE = 1024 * 1024 * 2 // The max size of a transaction to prevent DOS attacks

type Transaction struct {
	Version  byte
	TxType   TransactionType
	Nonce    uint32
	GasPrice uint64
	GasLimit uint64
	Payer    common.Address
	Payload  Payload
	//Attributes []*TxAttribute
	Attributes byte //this must be 0 now, Attribute Array length use VarUint encoding, so byte is enough for extension
	Sigs       []*Sig

	Raw []byte // raw transaction data

	hash       *common.Uint256
	SignedAddr []common.Address // this is assigned when passed signature verification

	nonDirectConstracted bool // used to check literal construction like `tx := &Transaction{...}`
}

// if no error, ownership of param raw is transfered to Transaction
func TransactionFromRawBytes(raw []byte) (*Transaction, error) {
	if len(raw) > MAX_TX_SIZE {
		return nil, errors.New("execced max transaction size")
	}
	source := common.NewZeroCopySource(raw)
	tx := &Transaction{Raw: raw}
	err := tx.Deserialization(source)
	if err != nil {
		return nil, err
	}
	return tx, nil
}

// Transaction has internal reference of param `source`
func (tx *Transaction) Deserialization(source *common.ZeroCopySource) error {
	pstart := source.Pos()
	err := tx.deserializationUnsigned(source)
	if err != nil {
		return err
	}
	pos := source.Pos()
	lenUnsigned := pos - pstart
	source.BackUp(lenUnsigned)
	rawUnsigned, _ := source.NextBytes(lenUnsigned)
	temp := sha256.Sum256(rawUnsigned)
	f := common.Uint256(sha256.Sum256(temp[:]))
	tx.hash = &f

	// tx sigs
	length, _, irregular, eof := source.NextVarUint()
	if irregular {
		return common.ErrIrregularData
	}
	if eof {
		return io.ErrUnexpectedEOF
	}
	if length > constants.TX_MAX_SIG_SIZE {
		return fmt.Errorf("transaction signature number %d execced %d", length, constants.TX_MAX_SIG_SIZE)
	}

	for i := 0; i < int(length); i++ {
		var rawsig RawSig
		err := rawsig.Deserialization(source)
		if err != nil {
			return err
		}
		sig, err := rawsig.GetSig()
		if err != nil {
			return err
		}
		tx.Sigs = append(tx.Sigs, &sig)
	}

	pend := source.Pos()
	lenAll := pend - pstart
	source.BackUp(lenAll)
	tx.Raw, _ = source.NextBytes(lenAll)

	tx.nonDirectConstracted = true

	return nil
}

// note: ownership transfered to output
func (tx *Transaction) IntoMutable() (*MutableTransaction, error) {
	mutable := &MutableTransaction{
		Version:  tx.Version,
		TxType:   tx.TxType,
		Nonce:    tx.Nonce,
		GasPrice: tx.GasPrice,
		GasLimit: tx.GasLimit,
		Payer:    tx.Payer,
		Payload:  tx.Payload,
	}

	for _, sig := range tx.Sigs {
		mutable.Sigs = append(mutable.Sigs, *sig)
	}

	return mutable, nil
}

func (tx *Transaction) deserializationUnsigned(source *common.ZeroCopySource) error {
	var irregular, eof bool
	tx.Version, eof = source.NextByte()
	var txtype byte
	txtype, eof = source.NextByte()
	tx.TxType = TransactionType(txtype)
	tx.Nonce, eof = source.NextUint32()
	tx.GasPrice, eof = source.NextUint64()
	tx.GasLimit, eof = source.NextUint64()
	var buf []byte
	buf, eof = source.NextBytes(common.ADDR_LEN)
	if eof {
		return io.ErrUnexpectedEOF
	}
	copy(tx.Payer[:], buf)

	switch tx.TxType {
	case Invoke:
		pl := new(payload.InvokeCode)
		err := pl.Deserialization(source)
		if err != nil {
			return err
		}
		tx.Payload = pl
	case Deploy:
		pl := new(payload.DeployCode)
		err := pl.Deserialization(source)
		if err != nil {
			return err
		}
		tx.Payload = pl
	default:
		return fmt.Errorf("unsupported tx type %v", tx.Type())
	}

	var length uint64
	var attr uint64
	attr, length, irregular, eof = source.NextVarUint()
	if irregular {
		return common.ErrIrregularData
	}
	if eof {
		return io.ErrUnexpectedEOF
	}

	if length != 1 {
		return fmt.Errorf("transaction attribute must be 0, got %d", length)
	}
	tx.Attributes = byte(attr)
	return nil
}

type RawSig struct {
	Invoke []byte
	Verify []byte
}

func (self *RawSig) Serialization(sink *common.ZeroCopySink) error {
	sink.WriteVarBytes(self.Invoke)
	sink.WriteVarBytes(self.Verify)
	return nil
}

func (self *RawSig) Serialize(w io.Writer) error {
	err := serialization.WriteVarBytes(w, self.Invoke)
	if err != nil {
		return err
	}
	err = serialization.WriteVarBytes(w, self.Verify)
	if err != nil {
		return err
	}

	return nil
}

func (self *RawSig) Deserialize(r io.Reader) error {
	invoke, err := serialization.ReadVarBytes(r)
	if err != nil {
		return err
	}
	verify, err := serialization.ReadVarBytes(r)
	if err != nil {
		return err
	}
	self.Invoke = invoke
	self.Verify = verify

	return nil
}

func (self *RawSig) Deserialization(source *common.ZeroCopySource) error {
	var eof, irregular bool
	self.Invoke, _, irregular, eof = source.NextVarBytes()
	if irregular {
		return common.ErrIrregularData
	}
	self.Verify, _, irregular, eof = source.NextVarBytes()
	if irregular {
		return common.ErrIrregularData
	}

	if eof {
		return io.ErrUnexpectedEOF
	}

	return nil
}

type Sig struct {
	SigData [][]byte
	PubKeys []keypair.PublicKey
	M       uint16
}

func (self *Sig) GetRawSig() (*RawSig, error) {
	invocationScript := program.ProgramFromParams(self.SigData)
	var verificationScript []byte
	if len(self.PubKeys) == 0 {
		return nil, errors.New("no pubkeys in sig")
	} else if len(self.PubKeys) == 1 {
		verificationScript = program.ProgramFromPubKey(self.PubKeys[0])
	} else {
		script, err := program.ProgramFromMultiPubKey(self.PubKeys, int(self.M))
		if err != nil {
			return nil, err
		}
		verificationScript = script
	}

	return &RawSig{Invoke: invocationScript, Verify: verificationScript}, nil
}

func (self *RawSig) GetSig() (Sig, error) {
	sigs, err := program.GetParamInfo(self.Invoke)
	if err != nil {
		return Sig{}, err
	}
	info, err := program.GetProgramInfo(self.Verify)
	if err != nil {
		return Sig{}, err
	}

	return Sig{SigData: sigs, M: info.M, PubKeys: info.PubKeys}, nil
}

func (self *Sig) Serialize(w io.Writer) error {
	invocationScript := program.ProgramFromParams(self.SigData)
	var verificationScript []byte
	if len(self.PubKeys) == 0 {
		return errors.New("no pubkeys in sig")
	} else if len(self.PubKeys) == 1 {
		verificationScript = program.ProgramFromPubKey(self.PubKeys[0])
	} else {
		script, err := program.ProgramFromMultiPubKey(self.PubKeys, int(self.M))
		if err != nil {
			return err
		}
		verificationScript = script
	}
	err := serialization.WriteVarBytes(w, invocationScript)
	if err != nil {
		return err
	}
	err = serialization.WriteVarBytes(w, verificationScript)
	if err != nil {
		return err
	}

	return nil
}

func (self *Sig) Deserialize(r io.Reader) error {
	invocationScript, err := serialization.ReadVarBytes(r)
	if err != nil {
		return err
	}
	verificationScript, err := serialization.ReadVarBytes(r)
	if err != nil {
		return err
	}
	sigs, err := program.GetParamInfo(invocationScript)
	if err != nil {
		return err
	}
	info, err := program.GetProgramInfo(verificationScript)
	if err != nil {
		return err
	}

	self.SigData = sigs
	self.M = info.M
	self.PubKeys = info.PubKeys

	return nil
}

func (self *Transaction) GetSignatureAddresses() []common.Address {
	address := make([]common.Address, 0, len(self.Sigs))
	for _, sig := range self.Sigs {
		m := int(sig.M)
		n := len(sig.PubKeys)
		if n == 1 {
			address = append(address, AddressFromPubKey(sig.PubKeys[0]))
		} else {
			addr, err := AddressFromMultiPubKeys(sig.PubKeys, m)
			if err != nil {
				return nil
			}
			address = append(address, addr)
		}
	}
	return address
}

type TransactionType byte

const (
	Bookkeeper TransactionType = 0x02
	Deploy     TransactionType = 0xd0
	Invoke     TransactionType = 0xd1
)

// Payload define the func for loading the payload data
// base on payload type which have different struture
type Payload interface {
	//Serialize payload data
	Serialize(w io.Writer) error

	Deserialize(r io.Reader) error
}

func (tx *Transaction) Serialization(sink *common.ZeroCopySink) error {
	if tx.nonDirectConstracted == false || len(tx.Raw) == 0 {
		panic("wrong constructed transaction")
	}
	sink.WriteBytes(tx.Raw)
	return nil
}

// Serialize the Transaction
/*
func (tx *Transaction) Serialize(w io.Writer) error {
	_, err := w.Write(tx.Raw)
	return err
}
*/

func (tx *Transaction) Serialize(w io.Writer) error {
	err := tx.SerializeUnsigned(w)
	if err != nil {
		return err
	}
	err = serialization.WriteVarUint(w, uint64(len(tx.Sigs)))
	if err != nil {
		return err
	}
	for _, sig := range tx.Sigs {
		err = sig.Serialize(w)
		if err != nil {
			return err
		}
	}
	return nil
}

//Serialize the Transaction data without contracts
func (tx *Transaction) SerializeUnsigned(w io.Writer) error {
	//txType
	if _, err := w.Write([]byte{byte(tx.Version), byte(tx.TxType)}); err != nil {
		return fmt.Errorf("[SerializeUnsigned], Transaction version failed. %v", err)
	}
	if err := serialization.WriteUint32(w, tx.Nonce); err != nil {
		return fmt.Errorf("[SerializeUnsigned], Transaction nonce failed. %v", err)
	}
	if err := serialization.WriteUint64(w, tx.GasPrice); err != nil {
		return fmt.Errorf("[SerializeUnsigned], Transaction gasPrice failed. %v", err)
	}
	if err := serialization.WriteUint64(w, tx.GasLimit); err != nil {
		return fmt.Errorf("[SerializeUnsigned], Transaction gasLimit failed. %v", err)
	}
	if err := tx.Payer.Serialize(w); err != nil {
		return fmt.Errorf("[SerializeUnsigned], Transaction payer failed. %v", err)
	}
	//Payload
	if tx.Payload == nil {
		return errors.New("Transaction Payload is nil.")
	}
	if err := tx.Payload.Serialize(w); err != nil {
		return fmt.Errorf("[SerializeUnsigned], Transaction payload failed. %v", err)
	}
	//err := serialization.WriteByte(w, tx.Attributes)
	err := serialization.WriteVarUint(w, uint64(tx.Attributes))
	if err != nil {
		return fmt.Errorf("[SerializeUnsigned], Transaction item txAttribute length serialization failed. %v", err)
	}

	return nil
}

// deserialize the Transaction
func (tx *Transaction) Deserialize(r io.Reader) error {
	// tx deserialize
	err := tx.DeserializeUnsigned(r)
	if err != nil {
		return fmt.Errorf("[Deserialize], Transaction deserializeUnsigned error. %v", err)
	}

	// tx sigs
	length, err := serialization.ReadVarUint(r, 0)
	if err != nil {
		return fmt.Errorf("[Deserialize], Transaction sigs length deserialize error. %v", err)
	}

	if length > constants.TX_MAX_SIG_SIZE {
		return fmt.Errorf("transaction signature number %d execced %d", length, constants.TX_MAX_SIG_SIZE)
	}

	for i := 0; i < int(length); i++ {
		sig := new(Sig)
		err := sig.Deserialize(r)
		if err != nil {
			return errors.New("deserialize transaction failed")
		}
		tx.Sigs = append(tx.Sigs, sig)
	}

	return nil
}

func (tx *Transaction) DeserializeUnsigned(r io.Reader) error {
	var versiontype [2]byte
	_, err := io.ReadFull(r, versiontype[:])
	if err != nil {
		return err
	}
	nonce, err := serialization.ReadUint32(r)
	if err != nil {
		return err
	}
	gasPrice, err := serialization.ReadUint64(r)
	if err != nil {
		return err
	}
	gasLimit, err := serialization.ReadUint64(r)
	if err != nil {
		return err
	}
	tx.Version = versiontype[0]
	tx.TxType = TransactionType(versiontype[1])
	tx.Nonce = nonce
	tx.GasPrice = gasPrice
	tx.GasLimit = gasLimit
	if err := tx.Payer.Deserialize(r); err != nil {
		return err
	}

	switch tx.TxType {
	case Invoke:
		tx.Payload = new(payload.InvokeCode)
	case Deploy:
		tx.Payload = new(payload.DeployCode)
	default:
		return errors.New(fmt.Sprintf("unsupported tx type %v", tx.Type()))
	}

	err = tx.Payload.Deserialize(r)
	if err != nil {
		return fmt.Errorf("[DeserializeUnsigned], Transaction payload parse error. %v", err)
	}

	//attributes
	attr, err := serialization.ReadVarUint(r, 0)
	if err != nil {
		return err
	}
	/*if length != 0 {
		return fmt.Errorf("transaction attribute must be 0, got %d", length)
	}*/
	tx.Attributes = byte(attr)

	return nil
}

func (tx *Transaction) GetMessage() []byte {
	buf := new(bytes.Buffer)
	tx.SerializeUnsigned(buf)
	return buf.Bytes()
}

func (tx *Transaction) ToArray() []byte {
	b := new(bytes.Buffer)
	tx.Serialize(b)
	return b.Bytes()
}

func (tx *Transaction) Hash() common.Uint256 {
	if tx.hash == nil {
		buf := bytes.Buffer{}
		tx.SerializeUnsigned(&buf)

		temp := sha256.Sum256(buf.Bytes())
		f := common.Uint256(sha256.Sum256(temp[:]))
		tx.hash = &f
	}
	return *tx.hash
}

func (tx *Transaction) Type() common.InventoryType {
	return common.TRANSACTION
}

func (tx *Transaction) Verify() error {
	panic("unimplemented ")
	return nil
}
