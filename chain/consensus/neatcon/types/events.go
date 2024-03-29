package types

import (
	. "github.com/neatio-net/common-go"
	"github.com/neatio-net/events-go"
	neatTypes "github.com/neatio-net/neatio/chain/core/types"
	"github.com/neatio-net/wire-go"
)

func EventStringBond() string    { return "Bond" }
func EventStringUnbond() string  { return "Unbond" }
func EventStringRebond() string  { return "Rebond" }
func EventStringDupeout() string { return "Dupeout" }
func EventStringFork() string    { return "Fork" }
func EventStringTx(tx Tx) string { return Fmt("Tx:%X", tx.Hash()) }

func EventStringNewBlock() string           { return "NewBlock" }
func EventStringNewBlockHeader() string     { return "NewBlockHeader" }
func EventStringNewRound() string           { return "NewRound" }
func EventStringNewRoundStep() string       { return "NewRoundStep" }
func EventStringTimeoutPropose() string     { return "TimeoutPropose" }
func EventStringCompleteProposal() string   { return "CompleteProposal" }
func EventStringPolka() string              { return "Polka" }
func EventStringUnlock() string             { return "Unlock" }
func EventStringLock() string               { return "Lock" }
func EventStringRelock() string             { return "Relock" }
func EventStringTimeoutWait() string        { return "TimeoutWait" }
func EventStringVote() string               { return "Vote" }
func EventStringSignAggr() string           { return "SignAggr" }
func EventStringVote2Proposer() string      { return "Vote2Proposer" }
func EventStringProposal() string           { return "Proposal" }
func EventStringBlockPart() string          { return "BlockPart" }
func EventStringProposalBlockParts() string { return "Proposal_BlockParts" }

func EventStringRequest() string        { return "Request" }
func EventStringMessage() string        { return "Message" }
func EventStringFinalCommitted() string { return "FinalCommitted" }

type TMEventData interface {
	events.EventData
	AssertIsTMEventData()
}

const (
	EventDataTypeNewBlock       = byte(0x01)
	EventDataTypeFork           = byte(0x02)
	EventDataTypeTx             = byte(0x03)
	EventDataTypeNewBlockHeader = byte(0x04)

	EventDataTypeRoundState    = byte(0x11)
	EventDataTypeVote          = byte(0x12)
	EventDataTypeSignAggr      = byte(0x13)
	EventDataTypeVote2Proposer = byte(0x14)

	EventDataTypeRequest        = byte(0x21)
	EventDataTypeMessage        = byte(0x22)
	EventDataTypeFinalCommitted = byte(0x23)
)

var _ = wire.RegisterInterface(
	struct{ TMEventData }{},
	wire.ConcreteType{EventDataNewBlock{}, EventDataTypeNewBlock},
	wire.ConcreteType{EventDataNewBlockHeader{}, EventDataTypeNewBlockHeader},

	wire.ConcreteType{EventDataTx{}, EventDataTypeTx},
	wire.ConcreteType{EventDataRoundState{}, EventDataTypeRoundState},
	wire.ConcreteType{EventDataVote{}, EventDataTypeVote},
	wire.ConcreteType{EventDataSignAggr{}, EventDataTypeSignAggr},
	wire.ConcreteType{EventDataVote2Proposer{}, EventDataTypeVote2Proposer},

	wire.ConcreteType{EventDataRequest{}, EventDataTypeRequest},
	wire.ConcreteType{EventDataMessage{}, EventDataTypeMessage},
	wire.ConcreteType{EventDataFinalCommitted{}, EventDataTypeFinalCommitted},
)

type EventDataNewBlock struct {
	Block *NCBlock `json:"block"`
}

type EventDataNewBlockHeader struct {
	Height int `json:"height"`
}

type EventDataTx struct {
	Height int    `json:"height"`
	Tx     Tx     `json:"tx"`
	Data   []byte `json:"data"`
	Log    string `json:"log"`
	Error  string `json:"error"`
}

type EventDataRoundState struct {
	Height uint64 `json:"height"`
	Round  int    `json:"round"`
	Step   string `json:"step"`

	RoundState interface{} `json:"-"`
}

type EventDataVote struct {
	Vote *Vote
}

type EventDataSignAggr struct {
	SignAggr *SignAggr
}

type EventDataVote2Proposer struct {
	Vote        *Vote
	ProposerKey string
}

type EventDataRequest struct {
	Proposal *neatTypes.Block `json:"proposal"`
}

type EventDataMessage struct {
	Payload []byte `json:"payload"`
}

type EventDataFinalCommitted struct {
	BlockNumber uint64
}

func (_ EventDataNewBlock) AssertIsTMEventData()       {}
func (_ EventDataNewBlockHeader) AssertIsTMEventData() {}
func (_ EventDataTx) AssertIsTMEventData()             {}
func (_ EventDataRoundState) AssertIsTMEventData()     {}
func (_ EventDataVote) AssertIsTMEventData()           {}
func (_ EventDataSignAggr) AssertIsTMEventData()       {}
func (_ EventDataVote2Proposer) AssertIsTMEventData()  {}

func (_ EventDataRequest) AssertIsTMEventData()        {}
func (_ EventDataMessage) AssertIsTMEventData()        {}
func (_ EventDataFinalCommitted) AssertIsTMEventData() {}

type Fireable interface {
	events.Fireable
}

type Eventable interface {
	SetEventSwitch(EventSwitch)
}

type EventSwitch interface {
	events.EventSwitch
}

type EventCache interface {
	Fireable
	Flush()
}

func NewEventSwitch() EventSwitch {
	return events.NewEventSwitch()
}

func NewEventCache(evsw EventSwitch) EventCache {
	return events.NewEventCache(evsw)
}

func fireEvent(fireable events.Fireable, event string, data TMEventData) {
	if fireable != nil {
		fireable.FireEvent(event, data)
	}
}

func AddListenerForEvent(evsw EventSwitch, id, event string, cb func(data TMEventData)) {
	evsw.AddListenerForEvent(id, event, func(data events.EventData) {
		cb(data.(TMEventData))
	})

}

func FireEventNewBlock(fireable events.Fireable, block EventDataNewBlock) {
	fireEvent(fireable, EventStringNewBlock(), block)
}

func FireEventNewBlockHeader(fireable events.Fireable, header EventDataNewBlockHeader) {
	fireEvent(fireable, EventStringNewBlockHeader(), header)
}

func FireEventVote(fireable events.Fireable, vote EventDataVote) {
	fireEvent(fireable, EventStringVote(), vote)
}

func FireEventSignAggr(fireable events.Fireable, sign EventDataSignAggr) {
	fireEvent(fireable, EventStringSignAggr(), sign)
}

func FireEventVote2Proposer(fireable events.Fireable, vote EventDataVote2Proposer) {
	fireEvent(fireable, EventStringVote2Proposer(), vote)
}

func FireEventTx(fireable events.Fireable, tx EventDataTx) {
	fireEvent(fireable, EventStringTx(tx.Tx), tx)
}

func FireEventNewRoundStep(fireable events.Fireable, rs EventDataRoundState) {
	fireEvent(fireable, EventStringNewRoundStep(), rs)
}

func FireEventTimeoutPropose(fireable events.Fireable, rs EventDataRoundState) {
	fireEvent(fireable, EventStringTimeoutPropose(), rs)
}

func FireEventTimeoutWait(fireable events.Fireable, rs EventDataRoundState) {
	fireEvent(fireable, EventStringTimeoutWait(), rs)
}

func FireEventNewRound(fireable events.Fireable, rs EventDataRoundState) {
	fireEvent(fireable, EventStringNewRound(), rs)
}

func FireEventCompleteProposal(fireable events.Fireable, rs EventDataRoundState) {
	fireEvent(fireable, EventStringCompleteProposal(), rs)
}

func FireEventPolka(fireable events.Fireable, rs EventDataRoundState) {
	fireEvent(fireable, EventStringPolka(), rs)
}

func FireEventUnlock(fireable events.Fireable, rs EventDataRoundState) {
	fireEvent(fireable, EventStringUnlock(), rs)
}

func FireEventRelock(fireable events.Fireable, rs EventDataRoundState) {
	fireEvent(fireable, EventStringRelock(), rs)
}

func FireEventLock(fireable events.Fireable, rs EventDataRoundState) {
	fireEvent(fireable, EventStringLock(), rs)
}

func FireEventRequest(fireable events.Fireable, rs EventDataRequest) {
	fireEvent(fireable, EventStringRequest(), rs)
}

func FireEventMessage(fireable events.Fireable, rs EventDataMessage) {
	fireEvent(fireable, EventStringMessage(), rs)
}

func FireEventFinalCommitted(fireable events.Fireable, rs EventDataFinalCommitted) {
	fireEvent(fireable, EventStringFinalCommitted(), rs)
}
