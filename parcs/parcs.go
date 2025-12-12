package parcs

import (
	"bytes"
	"encoding/xml"
	"fmt"
	"io"
	"time"
)

// Transaction defines a transaction after being read from NetSuite, but before writing to XML.
type Transaction struct {
	NetSuiteID           string
	CustomerExternalID   string
	Memo                 string
	SubsidiaryExternalID string
	TranDate             time.Time
	TranID               string
	Amount               int
	ParCSReference       string
	CustomerCategory     string
	ParCSTranCode        string
	TranType             string
}

type SubsidiaryTransactions struct {
	Subsidiary   string
	TotalAmount  int
	Transactions []Transaction
}

// PMISBatch is the definition of the top-level object in the XML file output.
type PMISBatch struct {
	XMLName xml.Name   `xml:"PMISBatch"`
	Header  PMISHeader `xml:"Header"`
	Trans   []PMISTran `xml:"PMISTran"`
}

// PMISHeader is the definition of the header object in the XML file.
type PMISHeader struct {
	BatchCount    int     `xml:"BatchCount"`
	BatchTotal    float32 `xml:"BatchTotal"`
	OriginatingPP string  `xml:"Originating_PP"`
}

// PMISTran is the definition of the transaction object in the XML file.
type PMISTran struct {
	TranType             string  `xml:"TranType"`
	RPP                  string  `xml:"RPP"`
	OPPTransactionAmount float64 `xml:"OPP_Transaction_Amount"`
	TransactionDesc      string  `xml:"Transaction_Description"`
	HouseholdCode        string  `xml:"Household_Code"`
	RPPDestination       string  `xml:"RPP_Destination_String"`
	RPPTranCode          string  `xml:"RPP_Trans_Type_Code"`
	OPPTransactionRef    string  `xml:"OPP_Transaction_Ref"`
	OriginatingPerson    string  `xml:"Originating_Person"`
	OPPTransactionDate   string  `xml:"OPP_Transaction_Date"`
}

// XMLDocument represents a single XML document.
type XMLDocument struct {
	Name    string `json:"filename"`
	Content string `json:"content"`
}

// CreateXMLDocuments converts a list of SubsidiaryTransactions to a list of XMLDocument
func CreateXMLDocuments(st []SubsidiaryTransactions) ([]XMLDocument, error) {
	today := time.Now().Format(time.RFC3339)

	docs := make([]XMLDocument, len(st))
	for i, t := range st {
		b, err := CreateXMLDocument(t)
		if err != nil {
			return nil, fmt.Errorf("XML error on %s: %w", t.Subsidiary, err)
		}

		docs[i] = XMLDocument{
			Name:    fmt.Sprintf("%s_%s.xml", t.Subsidiary, today),
			Content: string(b),
		}
	}
	return docs, nil
}

// CreateXMLDocument converts a SubsidiaryTransactions to an XMLDocument
func CreateXMLDocument(t SubsidiaryTransactions) ([]byte, error) {
	var w bytes.Buffer
	err := WriteXML(t, &w)
	if err != nil {
		return nil, fmt.Errorf("failed to create XML: %w", err)
	}
	return w.Bytes(), nil
}

// WriteXML creates XML data from a SubsidiaryTransactions batch.
func WriteXML(st SubsidiaryTransactions, w io.Writer) error {
	batch := PMISBatch{
		Header: PMISHeader{
			BatchCount:    len(st.Transactions),
			BatchTotal:    float32(st.TotalAmount) / 100,
			OriginatingPP: st.Subsidiary,
		},
		Trans: make([]PMISTran, len(st.Transactions)),
	}

	for i, t := range st.Transactions {
		batch.Trans[i] = ConvertTransaction(t)
	}

	enc := xml.NewEncoder(w)
	enc.Indent("", "\t")
	if _, err := io.WriteString(w, xml.Header); err != nil {
		return fmt.Errorf("failed writing XML header: %w", err)
	}

	if err := enc.Encode(batch); err != nil {
		return fmt.Errorf("xml encode failure: %w", err)
	}

	return nil
}

// ConvertTransaction makes a PMISTran from a Transaction for the XML generation process.
func ConvertTransaction(t Transaction) PMISTran {
	tranType := "GT"
	rpp := ""
	if t.CustomerCategory == "2" || t.CustomerCategory == "12" {
		rpp = t.CustomerExternalID
	}
	desc := t.Memo
	hhCode := ""
	if t.CustomerCategory == "10" || t.CustomerCategory == "7" {
		hhCode = t.CustomerExternalID[0:min(len(t.CustomerExternalID), 6)]
	}
	rppDest := t.ParCSReference
	rppCode := t.ParCSTranCode
	oppRef := fmt.Sprintf("Netsuite: %s_%s", t.TranType, t.TranID)
	origPers := "OppExport_Workday"
	oppDate := t.TranDate.Format(time.DateOnly)

	return PMISTran{
		TranType:             tranType,
		RPP:                  rpp,
		OPPTransactionAmount: float64(t.Amount) / 100,
		TransactionDesc:      desc,
		HouseholdCode:        hhCode,
		RPPDestination:       rppDest,
		RPPTranCode:          rppCode,
		OPPTransactionRef:    oppRef,
		OriginatingPerson:    origPers,
		OPPTransactionDate:   oppDate,
	}
}
