package agx_test

import (
	"github.com/rcgoodfellow/agx"
	"reflect"
	"testing"
)

//tests ~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~

// +++ OpenMessage +++
func TestMarshalOpenMessage(t *testing.T) {

	id, descr := "1.2.3.4.7", "muffin man"
	a, err := agx.NewOpenMessage(&id, &descr)
	if err != nil {
		t.Fatalf("error creating open message %v ", err)
	}
	b := &agx.OpenMessage{}
	roundTripTest(t, a, b)
}

// +++ CloseMessage +++
func TestMarshalCloseMessage(t *testing.T) {
	a := agx.NewCloseMessage(agx.CloseReasonShutdown, 47)
	b := &agx.CloseMessage{}
	roundTripTest(t, a, b)
}

// +++ RegisterMessage +++
func TestMarshalRegisterMessage(t *testing.T) {
	context := "pirates"
	a, err := agx.NewRegisterMessage("1.2.3.4.7", &context, nil)
	if err != nil {
		t.Fatalf("error creating register message %v ", err)
	}
	b := &agx.RegisterMessage{}
	roundTripTest(t, a, b)
}

//helpers =====================================================================

func roundTripTest(t *testing.T, a, b agx.AgentXMessage) {
	buf, err := a.MarshalBinary()
	if err != nil {
		t.Fatalf("error marshalling message %v ", err)
	}
	_, err = b.UnmarshalBinary(buf)
	if err != nil {
		t.Fatalf("error unmarshalling message %v ", err)
	}
	t.Logf("%v", a)
	t.Logf("%v", b)
	if !reflect.DeepEqual(a, b) {
		t.Errorf("structs are not equal")
	}

}
