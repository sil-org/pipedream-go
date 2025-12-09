package parcs_test

import (
	"bytes"
	"testing"
	"time"

	"github.com/google/go-cmp/cmp"

	"github.com/sil-org/pipedream-go/parcs"
)

const xmlSample = `<?xml version="1.0" encoding="UTF-8"?>
<PMISBatch>
	<Header>
		<BatchCount>2</BatchCount>
		<BatchTotal>111</BatchTotal>
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
		<OPP_Transaction_Amount>99.9</OPP_Transaction_Amount>
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
	SaleAmount:           1110,
	RefundAmount:         0,
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
	SaleAmount:           0,
	RefundAmount:         9990,
	ParCSReference:       "ref1",
	CustomerCategory:     "10",
	ParCSTranCode:        "MC",
	TranType:             "CashRfnd",
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
				OPPTransactionAmount: 99.9,
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
		TotalAmount:  cashSale.SaleAmount + cashRefund.RefundAmount,
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
