package consensus

import (
	"github.com/neatlab/neatio/chain/consensus/neatcon/types"
)

func subscribeToEvent(evsw types.EventSwitch, receiver, eventID string, chanCap int) chan interface{} {

	ch := make(chan interface{}, chanCap)
	types.AddListenerForEvent(evsw, receiver, eventID, func(data types.TMEventData) {
		ch <- data
	})
	return ch
}

func subscribeToEventRespond(evsw types.EventSwitch, receiver, eventID string) chan interface{} {

	ch := make(chan interface{})
	types.AddListenerForEvent(evsw, receiver, eventID, func(data types.TMEventData) {
		ch <- data
		<-ch
	})
	return ch
}
