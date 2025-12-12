package parcs_test

import (
	"bytes"
	"strings"
	"testing"
	"time"

	"github.com/google/go-cmp/cmp"
	"github.com/google/go-cmp/cmp/cmpopts"

	"github.com/sil-org/pipedream-go/parcs"
)

const xmlSample = `<?xml version="1.0" encoding="UTF-8"?>
<PMISBatch>
	<Header>
		<BatchCount>2</BatchCount>
		<BatchTotal>-88.8</BatchTotal>
		<Originating_PP>XYZ</Originating_PP>
	</Header>
	<PMISTran>
		<TranType>GT</TranType>
		<RPP></RPP>
		<OPP_Transaction_Amount>11.1</OPP_Transaction_Amount>
		<Transaction_Description>Sample Transaction Description</Transaction_Description>
		<Household_Code>223944</Household_Code>
		<RPP_Destination_String>ref1</RPP_Destination_String>
		<RPP_Trans_Type_Code>MC</RPP_Trans_Type_Code>
		<OPP_Transaction_Ref>Netsuite: CashSale_CS90384</OPP_Transaction_Ref>
		<Originating_Person>OppExport_Workday</Originating_Person>
		<OPP_Transaction_Date>2025-07-31</OPP_Transaction_Date>
	</PMISTran>
	<PMISTran>
		<TranType>GT</TranType>
		<RPP></RPP>
		<OPP_Transaction_Amount>-99.9</OPP_Transaction_Amount>
		<Transaction_Description>Sample description with &lt;brackets&gt; and &#39;quotes&#39;</Transaction_Description>
		<Household_Code>223944</Household_Code>
		<RPP_Destination_String>ref1</RPP_Destination_String>
		<RPP_Trans_Type_Code>MC</RPP_Trans_Type_Code>
		<OPP_Transaction_Ref>Netsuite: CashRfnd_CS90384</OPP_Transaction_Ref>
		<Originating_Person>OppExport_Workday</Originating_Person>
		<OPP_Transaction_Date>2025-07-31</OPP_Transaction_Date>
	</PMISTran>
</PMISBatch>`

var cashSale = parcs.Transaction{
	NetSuiteID:           "111111",
	CustomerExternalID:   "223944_XXX",
	Memo:                 "Sample Transaction Description",
	SubsidiaryExternalID: "",
	TranDate:             time.Date(2025, 7, 31, 0, 0, 0, 0, time.UTC),
	TranID:               "CS90384",
	Amount:               1110,
	ParCSReference:       "ref1",
	CustomerCategory:     "10",
	ParCSTranCode:        "MC",
	TranType:             "CashSale",
}

var cashRefund = parcs.Transaction{
	NetSuiteID:           "111111",
	CustomerExternalID:   "223944_XXX",
	Memo:                 "Sample description with <brackets> and 'quotes'",
	SubsidiaryExternalID: "",
	TranDate:             time.Date(2025, 7, 31, 0, 0, 0, 0, time.UTC),
	TranID:               "CS90384",
	Amount:               -9990,
	ParCSReference:       "ref1",
	CustomerCategory:     "10",
	ParCSTranCode:        "MC",
	TranType:             "CashRfnd",
}

func Test_createXMLDocuments(t *testing.T) {
	st := []parcs.SubsidiaryTransactions{{
		Subsidiary:   "XYZ",
		TotalAmount:  cashSale.Amount + cashRefund.Amount,
		Transactions: []parcs.Transaction{cashSale, cashRefund},
	}}

	want := []parcs.XMLDocument{{
		Name:    "XYZ",
		Content: xmlSample,
	}}

	got, err := parcs.CreateXMLDocuments(st)
	if err != nil {
		t.Errorf("createXMLDocuments() error = %v", err)
		return
	}
	if !strings.HasPrefix(got[0].Name, st[0].Subsidiary) {
		t.Error("XML document does not have the expected name, should start with the subsidiary code")
	}
	if !cmp.Equal(got[0], want[0], cmpopts.IgnoreFields(parcs.XMLDocument{}, "Name")) {
		t.Error("diff:", cmp.Diff(got[0], want[0]))
	}
}

func Test_createXMLDocument(t *testing.T) {
	tests := []struct {
		name    string
		st      parcs.SubsidiaryTransactions
		want    []byte
		wantErr bool
	}{
		{
			name: "empty",
			st:   parcs.SubsidiaryTransactions{},
			want: []byte(`<?xml version="1.0" encoding="UTF-8"?>
<PMISBatch>
	<Header>
		<BatchCount>0</BatchCount>
		<BatchTotal>0</BatchTotal>
		<Originating_PP></Originating_PP>
	</Header>
</PMISBatch>`),
			wantErr: false,
		},
		{
			name: "one",
			st: parcs.SubsidiaryTransactions{
				Subsidiary:  "x",
				TotalAmount: 1,
				Transactions: []parcs.Transaction{{
					NetSuiteID:           "a",
					CustomerExternalID:   "b",
					Memo:                 "c",
					SubsidiaryExternalID: "d",
					TranDate:             time.Date(2009, time.November, 10, 23, 0, 0, 0, time.UTC),
					TranID:               "e",
					Amount:               1,
					ParCSReference:       "f",
					CustomerCategory:     "g",
					ParCSTranCode:        "h",
					TranType:             "i",
				}},
			},
			want: []byte(`<?xml version="1.0" encoding="UTF-8"?>
<PMISBatch>
	<Header>
		<BatchCount>1</BatchCount>
		<BatchTotal>0.01</BatchTotal>
		<Originating_PP>x</Originating_PP>
	</Header>
	<PMISTran>
		<TranType>GT</TranType>
		<RPP></RPP>
		<OPP_Transaction_Amount>0.01</OPP_Transaction_Amount>
		<Transaction_Description>c</Transaction_Description>
		<Household_Code></Household_Code>
		<RPP_Destination_String>f</RPP_Destination_String>
		<RPP_Trans_Type_Code>h</RPP_Trans_Type_Code>
		<OPP_Transaction_Ref>Netsuite: i_e</OPP_Transaction_Ref>
		<Originating_Person>OppExport_Workday</Originating_Person>
		<OPP_Transaction_Date>2009-11-10</OPP_Transaction_Date>
	</PMISTran>
</PMISBatch>`),
			wantErr: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := parcs.CreateXMLDocument(tt.st)
			if (err != nil) != tt.wantErr {
				t.Errorf("error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if !cmp.Equal(got, tt.want) {
				t.Error("diff:", cmp.Diff(got, tt.want))
			}
		})
	}
}

func Test_convertTransaction(t *testing.T) {
	tests := []struct {
		name        string
		transaction parcs.Transaction
		want        parcs.PMISTran
	}{
		{
			name:        "CashSale",
			transaction: cashSale,
			want: parcs.PMISTran{
				TranType:             "GT",
				RPP:                  "",
				OPPTransactionAmount: 11.1,
				TransactionDesc:      "Sample Transaction Description",
				HouseholdCode:        "223944",
				RPPDestination:       "ref1",
				RPPTranCode:          "MC",
				OPPTransactionRef:    "Netsuite: CashSale_CS90384",
				OriginatingPerson:    "OppExport_Workday",
				OPPTransactionDate:   "2025-07-31",
			},
		},
		{
			name:        "CashRfnd",
			transaction: cashRefund,
			want: parcs.PMISTran{
				TranType:             "GT",
				RPP:                  "",
				OPPTransactionAmount: -99.9,
				TransactionDesc:      "Sample description with <brackets> and 'quotes'",
				HouseholdCode:        "223944",
				RPPDestination:       "ref1",
				RPPTranCode:          "MC",
				OPPTransactionRef:    "Netsuite: CashRfnd_CS90384",
				OriginatingPerson:    "OppExport_Workday",
				OPPTransactionDate:   "2025-07-31",
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := parcs.ConvertTransaction(tt.transaction); !cmp.Equal(got, tt.want) {
				t.Errorf("diff: %v", cmp.Diff(got, tt.want))
			}
		})
	}
}

func Test_writeXML(t *testing.T) {
	st := parcs.SubsidiaryTransactions{
		Subsidiary:   "XYZ",
		TotalAmount:  cashSale.Amount + cashRefund.Amount,
		Transactions: []parcs.Transaction{cashSale, cashRefund},
	}

	w := &bytes.Buffer{}
	err := parcs.WriteXML(st, w)
	if err != nil {
		t.Errorf("WriteXML() error = %v", err)
		return
	}
	got := w.String()
	if !cmp.Equal(got, xmlSample) {
		t.Errorf("diff: %v", cmp.Diff(got, xmlSample))
	}
}
