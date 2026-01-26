package parcs

import (
	"bytes"
	"net/http"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/google/go-cmp/cmp"
	"github.com/google/go-cmp/cmp/cmpopts"
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

var cashSale = Transaction{
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

var cashRefund = Transaction{
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

const sampleSearchResponse = `{"results":[` + sampleCashRefundJSON + "," + sampleCashSaleJSON + `]}`

const sampleCashRefundJSON = `{
    "recordType" : "cashrefund",
    "id" : "123456",
    "values" : {
      "internalid" : [ {
        "value" : "123456",
        "text" : "123456"
      } ],
      "type" : [ {
        "value" : "CashRfnd",
        "text" : "Cash Refund"
      } ],
      "subsidiarynohierarchy" : [ {
        "value" : "7",
        "text" : "Acme Corp"
      } ],
      "subsidiary" : [ {
        "value" : "7",
        "text" : "*Acme Corp : *Acme : Acme Corporation"
      } ],
      "trandate" : "08/01/2025",
      "postingperiod" : [ {
        "value" : "120",
        "text" : "Aug 2025"
      } ],
      "transactionnumber" : "CASHRFND3096",
      "tranid" : "RFND3096",
      "entity" : [ {
        "value" : "13406",
        "text" : "ParCS (777777), Smith, John & Jane"
      } ],
      "memo" : "Reverse Smith FY 2025 August ParCS Payment (CS22222)",
      "custbody_for_parcs" : true,
      "custcol_parcs_tran_type_code" : [ {
        "value" : "1",
        "text" : "MC"
      } ],
      "custcol1" : "",
      "custbody_parcs_code_body" : [ ],
      "custbody_parcs_ref_body" : "",
      "item.displayname" : "Facility Rental",
      "taxamount" : "",
      "netamountnotax" : "-730.00",
      "amount" : "-730.00",
      "creditamount" : "",
      "debitamount" : "730.00",
      "customer.internalid" : [ {
        "value" : "33333",
        "text" : "33333"
      } ],
      "customer.custentity_sil_cust_category" : [ {
        "value" : "10",
        "text" : "Acme Staff for ParCS"
      } ],
      "customer.entityid" : "ParCS (777777), Smith, John & Jane",
      "customer.externalid" : [ {
        "value" : "777777_C",
        "text" : "777777_C"
      } ],
      "subsidiary.custrecord155" : "ACC"
    }
  }`

const sampleCashSaleJSON = `{
    "recordType" : "cashsale",
    "id" : "987654",
    "values" : {
      "internalid" : [ {
        "value" : "987654",
        "text" : "987654"
      } ],
      "type" : [ {
        "value" : "CashSale",
        "text" : "Cash Sale/Donation"
      } ],
      "subsidiarynohierarchy" : [ {
        "value" : "7",
        "text" : "Acme Corporation"
      } ],
      "subsidiary" : [ {
        "value" : "7",
        "text" : "*Acme Corp : *Acme : Acme Corporation"
      } ],
      "trandate" : "08/13/2025",
      "postingperiod" : [ {
        "value" : "120",
        "text" : "Aug 2025"
      } ],
      "transactionnumber" : "CASHSALE95107",
      "tranid" : "CS95107",
      "entity" : [ {
        "value" : "6786",
        "text" : "Johnson Manufacturing-ParCS"
      } ],
      "memo" : "Johnson Manufacturing: JMC 11010 WEVACF\nAIG Travel Assist Inv Jack Johnson ",
      "custbody_for_parcs" : true,
      "custcol_parcs_tran_type_code" : [ ],
      "custcol1" : "",
      "custbody_parcs_code_body" : [ ],
      "custbody_parcs_ref_body" : "",
      "item.displayname" : "",
      "taxamount" : "",
      "netamountnotax" : "-70125.00",
      "amount" : "-70125.00",
      "creditamount" : "70125.00",
      "debitamount" : "",
      "customer.internalid" : [ {
        "value" : "6786",
        "text" : "6786"
      } ],
      "customer.custentity_sil_cust_category" : [ {
        "value" : "2",
        "text" : "Alliance Organization (ParCS)"
      } ],
      "customer.entityid" : "Johnson Manufacturing-ParCS",
      "customer.externalid" : [ {
        "value" : "JMC",
        "text" : "JMC"
      } ],
      "subsidiary.custrecord155" : "ACC"
    }
  }`

func sampleResponse() SearchResponse {
	return SearchResponse{
		Results: []SearchRecord{sampleCashRefund(), sampleCashSale()},
	}
}

func sampleCashRefund() SearchRecord {
	return SearchRecord{
		RecordType: "cashrefund",
		ID:         "123456",
		Values: Values{
			InternalID:                        []SelectValue{{Value: "123456", Text: "123456"}},
			Type:                              []SelectValue{{Value: "CashRfnd", Text: "Cash Refund"}},
			TransactionDate:                   "08/01/2025",
			TranID:                            "RFND3096",
			Memo:                              "Reverse Smith FY 2025 August ParCS Payment (CS22222)",
			CustcolParcsTranTypeCode:          []SelectValue{{Value: "1", Text: "MC"}},
			CustbodyParcsRefBody:              "",
			TaxAmount:                         "",
			CreditAmount:                      "",
			DebitAmount:                       "730.00",
			CustomerCustentitySILCustCategory: []SelectValue{{Value: "10", Text: "Acme Staff for ParCS"}},
			CustomerExternalID:                []SelectValue{{Value: "777777_C", Text: "777777_C"}},
			SubsidiaryCustRecord155:           "ACC",
		},
	}
}

func sampleCashSale() SearchRecord {
	return SearchRecord{
		RecordType: "cashsale",
		ID:         "987654",
		Values: Values{
			InternalID:                        []SelectValue{{Value: "987654", Text: "987654"}},
			Type:                              []SelectValue{{Value: "CashSale", Text: "Cash Sale/Donation"}},
			TransactionDate:                   "08/13/2025",
			TranID:                            "CS95107",
			Memo:                              "Johnson Manufacturing: JMC 11010 WEVACF\nAIG Travel Assist Inv Jack Johnson ",
			CustcolParcsTranTypeCode:          []SelectValue{},
			CustbodyParcsRefBody:              "",
			TaxAmount:                         "",
			CreditAmount:                      "70125.00",
			DebitAmount:                       "",
			CustomerCustentitySILCustCategory: []SelectValue{{Value: "2", Text: "Alliance Organization (ParCS)"}},
			CustomerExternalID:                []SelectValue{{Value: "JMC", Text: "JMC"}},
			SubsidiaryCustRecord155:           "ACC",
		},
	}
}

func Test_createXMLDocuments(t *testing.T) {
	st := []SubsidiaryTransactions{{
		Subsidiary:   "XYZ",
		TotalAmount:  cashSale.Amount + cashRefund.Amount,
		Transactions: []Transaction{cashSale, cashRefund},
	}}

	want := []XMLDocument{{
		Name:    "XYZ",
		Content: xmlSample,
	}}

	got, err := createXMLDocuments(st)
	if err != nil {
		t.Errorf("createXMLDocuments() error = %v", err)
		return
	}
	if !strings.HasPrefix(got[0].Name, st[0].Subsidiary) {
		t.Error("XML document does not have the expected name, should start with the subsidiary code")
	}
	if !cmp.Equal(got[0], want[0], cmpopts.IgnoreFields(XMLDocument{}, "Name")) {
		t.Error("diff:", cmp.Diff(got[0], want[0]))
	}
}

func Test_createXMLDocument(t *testing.T) {
	tests := []struct {
		name    string
		st      SubsidiaryTransactions
		want    []byte
		wantErr bool
	}{
		{
			name: "empty",
			st:   SubsidiaryTransactions{},
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
			st: SubsidiaryTransactions{
				Subsidiary:  "x",
				TotalAmount: 1,
				Transactions: []Transaction{{
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
			got, err := createXMLDocument(tt.st)
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
		transaction Transaction
		want        PMISTran
	}{
		{
			name:        "CashSale",
			transaction: cashSale,
			want: PMISTran{
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
			want: PMISTran{
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
			if got := convertTransaction(tt.transaction); !cmp.Equal(got, tt.want) {
				t.Errorf("diff: %v", cmp.Diff(got, tt.want))
			}
		})
	}
}

func Test_writeXML(t *testing.T) {
	st := SubsidiaryTransactions{
		Subsidiary:   "XYZ",
		TotalAmount:  cashSale.Amount + cashRefund.Amount,
		Transactions: []Transaction{cashSale, cashRefund},
	}

	w := &bytes.Buffer{}
	err := writeXML(st, w)
	if err != nil {
		t.Errorf("WriteXML() error = %v", err)
		return
	}
	got := w.String()
	if !cmp.Equal(got, xmlSample) {
		t.Errorf("diff: %v", cmp.Diff(got, xmlSample))
	}
}

func Test_firstValue(t *testing.T) {
	tests := []struct {
		name string
		v    []SelectValue
		want string
	}{
		{name: "nil", v: nil, want: ""},
		{name: "none", v: []SelectValue{}, want: ""},
		{name: "one", v: []SelectValue{{Value: "v", Text: "t"}}, want: "v"},
		{name: "two", v: []SelectValue{{Value: "v1", Text: "t1"}, {Value: "v2", Text: "t2"}}, want: "v1"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := firstValue(tt.v); got != tt.want {
				t.Errorf("firstValue() = %v, want %v", got, tt.want)
			}
		})
	}
}

func Test_firstText(t *testing.T) {
	tests := []struct {
		name string
		v    []SelectValue
		want string
	}{
		{name: "nil", v: nil, want: ""},
		{name: "none", v: []SelectValue{}, want: ""},
		{name: "one", v: []SelectValue{{Value: "v", Text: "t"}}, want: "t"},
		{name: "two", v: []SelectValue{{Value: "v1", Text: "t1"}, {Value: "v2", Text: "t2"}}, want: "t1"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := firstText(tt.v); got != tt.want {
				t.Errorf("firstText() = %v, want %v", got, tt.want)
			}
		})
	}
}

func Test_parseAmount(t *testing.T) {
	tests := []struct {
		s    string
		want int
	}{
		{s: "0", want: 0},
		{s: "10", want: 1000},
		{s: "-1", want: -100},
		{s: "0.01", want: 1},
		{s: "0.05", want: 5},
		{s: "0.10", want: 10},
		{s: "-0.01", want: -1},
		{s: "-0.05", want: -5},
		{s: "-0.10", want: -10},
		{s: ".01", want: 1},
		{s: "-.01", want: -1},
	}
	for _, tt := range tests {
		t.Run(tt.s, func(t *testing.T) {
			if got := parseAmount(tt.s); got != tt.want {
				t.Errorf("parseAmount() = %v, want %v", got, tt.want)
			}
		})
	}
}

func Test_validateConfig(t *testing.T) {
	fullConfig := Config{
		NetSuiteConsumerKey:    "x",
		NetSuiteConsumerSecret: "x",
		NetSuiteToken:          "x",
		NetSuiteTokenSecret:    "x",
		NetSuiteRealm:          "x",
		NetSuiteSavedSearchURL: "x",
		NetSuiteSearchID:       "x",
		SFTPUsername:           "x",
		SFTPHost:               "x",
		SFTPPrivateKey:         "x",
		SFTPDirectory:          "x",
		client:                 &http.Client{},
	}
	tests := []struct {
		name    string
		cfg     Config
		wantErr bool
	}{
		{name: "empty", cfg: Config{}, wantErr: true},
		{name: "partial", cfg: Config{SFTPHost: "x"}, wantErr: true},
		{name: "full", cfg: fullConfig, wantErr: false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if err := validateConfig(tt.cfg); (err != nil) != tt.wantErr {
				t.Errorf("validateConfig() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

func Test_decodeResponse(t *testing.T) {
	tests := []struct {
		name    string
		body    []byte
		want    SearchResponse
		wantErr string
	}{
		{
			name:    "nil body",
			body:    nil,
			want:    SearchResponse{},
			wantErr: "NetSuite API body failed to unmarshal: EOF",
		},
		{
			name:    "empty body",
			body:    []byte{},
			want:    SearchResponse{},
			wantErr: "NetSuite API body failed to unmarshal: EOF",
		},
		{
			name: "empty JSON",
			body: []byte("{}"),
			want: SearchResponse{},
		},
		{
			name: "sample",
			body: []byte(sampleSearchResponse),
			want: sampleResponse(),
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := decodeResponse(tt.body)
			if tt.wantErr != "" {
				if err == nil {
					t.Errorf("expected error: %v", tt.wantErr)
				}
				if tt.wantErr != err.Error() {
					t.Errorf("incorrect error: %v, expected: %v", err, tt.wantErr)
				}
				return
			}

			if err != nil {
				t.Errorf("unexpected error: %v", err)
				return
			}

			if !reflect.DeepEqual(got, tt.want) {
				t.Errorf("decodeResponse() got = %v, want %v", got, tt.want)
			}
		})
	}
}

func Test_transactionURL(t *testing.T) {
	const transactionID = "101"
	const realm = "1234567"

	tests := []struct {
		name            string
		transactionType string
		want            string
		wantErr         string
	}{
		{
			name:            "invalid type",
			transactionType: "x",
			wantErr:         "invalid transaction type: x",
		},
		{
			name:            "cash refund",
			transactionType: CashRefund,
			want:            "https://1234567.suitetalk.api.netsuite.com/services/rest/record/v1/cashRefund/101",
		},
		{
			name:            "cash sale",
			transactionType: CashSale,
			want:            "https://1234567.suitetalk.api.netsuite.com/services/rest/record/v1/cashSale/101",
		},
		{
			name:            "customer deposit",
			transactionType: CustomerDeposit,
			want:            "https://1234567.suitetalk.api.netsuite.com/services/rest/record/v1/customerDeposit/101",
		},
		{
			name:            "customer refund",
			transactionType: CustomerRefund,
			want:            "https://1234567.suitetalk.api.netsuite.com/services/rest/record/v1/customerRefund/101",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := transactionURL(transactionID, tt.transactionType, realm)
			if tt.wantErr != "" {
				if err == nil {
					t.Errorf("expected error: %v", tt.wantErr)
				}
				if tt.wantErr != err.Error() {
					t.Errorf("incorrect error: %v, expected: %v", err, tt.wantErr)
				}
				return
			}

			if err != nil {
				t.Errorf("unexpected error: %v", err)
				return
			}

			if got != tt.want {
				t.Errorf("transactionURL() got = %v, want %v", got, tt.want)
			}
		})
	}
}
