package parcs

import (
	"bytes"
	"context"
	"encoding/json"
	"encoding/xml"
	"errors"
	"fmt"
	"io"
	"log"
	"math"
	"net/http"
	"os"
	"reflect"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/dghubble/oauth1"
	"github.com/kelseyhightower/envconfig"
	"github.com/pkg/sftp"

	"github.com/sil-org/pipedream-go/config"
	"github.com/sil-org/pipedream-go/ssh"
)

// MaxConcurrent sets the number of concurrent transaction updates. Setting higher than 5 may cause a concurrency
// limit error: "Concurrent request limit exceeded. Request blocked. Verify your concurrency limits at Setup >
// Integration > Integration Management > Integration Governance."
const MaxConcurrent = 5

const (
	CashRefund      = "CashRfnd"
	CashSale        = "CashSale"
	CustomerDeposit = "CustDep"
	CustomerRefund  = "CustRfnd"
)

type Config struct {
	NetSuiteConsumerKey    string `split_words:"true"`
	NetSuiteConsumerSecret string `split_words:"true"`
	NetSuiteToken          string `split_words:"true"`
	NetSuiteTokenSecret    string `split_words:"true"`
	NetSuiteRealm          string `split_words:"true"`
	NetSuiteSavedSearchURL string `split_words:"true"`
	NetSuiteSearchID       string `split_words:"true"`
	SFTPUsername           string `split_words:"true"`
	SFTPHost               string `split_words:"true"`
	SFTPPrivateKey         string `split_words:"true"`
	SFTPDirectory          string `split_words:"true"`

	client *http.Client
}

type SearchResponse struct {
	Results []SearchRecord `json:"results"`
}

type SearchRecord struct {
	RecordType string `json:"recordType"`
	ID         string `json:"id"` // not used
	Values     Values `json:"values"`
}

type Values struct {
	InternalID                        []SelectValue `json:"internalid"`
	Type                              []SelectValue `json:"type"`
	TransactionDate                   string        `json:"trandate"`
	TranID                            string        `json:"tranid"`
	Memo                              string        `json:"memo"`
	CustcolParcsTranTypeCode          []SelectValue `json:"custcol_parcs_tran_type_code"`
	CustbodyParcsRefBody              string        `json:"custbody_parcs_ref_body"`
	TaxAmount                         string        `json:"taxamount"`
	CreditAmount                      string        `json:"creditamount"`
	DebitAmount                       string        `json:"debitamount"`
	CustomerCustentitySILCustCategory []SelectValue `json:"customer.custentity_sil_cust_category"`
	CustomerExternalID                []SelectValue `json:"customer.externalid"`
	SubsidiaryCustRecord155           string        `json:"subsidiary.custrecord155"`
}

type SelectValue struct {
	Value string `json:"value"`
	Text  string `json:"text"`
}

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

// GetConfig reads from AWS Systems Manager Parameter Store and returns a Config structure for use by other functions
// in this package.
func GetConfig(ctx context.Context) (Config, error) {
	var cfg Config
	config.ReadParameterStore(os.Getenv("PARAMETER_STORE_PREFIX"), &cfg)

	if err := envconfig.Process("", &cfg); err != nil {
		return Config{}, err
	}

	oauthConfig := oauth1.NewConfig(cfg.NetSuiteConsumerKey, cfg.NetSuiteConsumerSecret)
	oauthConfig.Realm = cfg.NetSuiteRealm
	oauthConfig.Signer = &oauth1.HMAC256Signer{ConsumerSecret: cfg.NetSuiteConsumerSecret}

	token := oauth1.NewToken(cfg.NetSuiteToken, cfg.NetSuiteTokenSecret)
	cfg.client = oauthConfig.Client(ctx, token)

	if err := validateConfig(cfg); err != nil {
		return Config{}, err
	}

	return cfg, nil
}

// validateConfig checks that all the config fields have been set. It doesn't ensure correctness, but merely that they
// are not blank.
func validateConfig(cfg Config) error {
	v := reflect.ValueOf(cfg)
	t := v.Type()
	if v.Kind() != reflect.Struct {
		return fmt.Errorf("expected struct got %s", t.Name())
	}

	var zeroFields []string
	for i := 0; i < v.NumField(); i++ {
		field := v.Field(i)
		if field.IsZero() {
			zeroFields = append(zeroFields, t.Field(i).Name)
		}
	}

	if len(zeroFields) > 0 {
		return fmt.Errorf("missing configuration for: %s", strings.Join(zeroFields, ", "))
	}
	return nil
}

// DoSavedSearch calls the configured NetSuite saved search and decodes the response to a SearchResponse.
func DoSavedSearch(ctx context.Context, cfg Config) (SearchResponse, error) {
	payload := strings.NewReader(fmt.Sprintf(`{"searchID":"%s"}`, cfg.NetSuiteSearchID))
	resp, err := cfg.client.Post(cfg.NetSuiteSavedSearchURL, "application/json", payload)
	if err != nil {
		return SearchResponse{}, fmt.Errorf("call failed: %w", err)
	}
	defer func() {
		err := resp.Body.Close()
		if err != nil {
			log.Fatal(err)
		}
	}()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return SearchResponse{}, fmt.Errorf("body read failed: %w", err)
	}

	if resp.StatusCode != 200 {
		return SearchResponse{}, fmt.Errorf("call failed with status code %d, body: %s", resp.StatusCode, body)
	}
	return decodeResponse(body)
}

// decodeResponse performs the JSON decode for the NetSuite ParCS saved search.
func decodeResponse(body []byte) (SearchResponse, error) {
	var response SearchResponse
	decoder := json.NewDecoder(bytes.NewReader(body))
	err := decoder.Decode(&response)
	if err != nil {
		return SearchResponse{}, fmt.Errorf("NetSuite API body failed to unmarshal: %w", err)
	}
	return response, nil
}

func TransformSearchResponse(response SearchResponse) []Transaction {
	rows := make([]Transaction, len(response.Results))
	for i, r := range response.Results {
		v := r.Values

		tranType := firstValue(v.Type)
		taxAmount := parseAmount(v.TaxAmount)
		creditAmount := parseAmount(v.CreditAmount)
		debitAmount := parseAmount(v.DebitAmount)

		amount := 0
		switch tranType {
		case CustomerDeposit:
			amount = creditAmount
		case CustomerRefund:
			amount = -debitAmount
		case CashSale:
			amount = creditAmount + taxAmount
		case CashRefund:
			amount = taxAmount - debitAmount
		}

		t := Transaction{}

		t.NetSuiteID = firstValue(v.InternalID)
		t.CustomerExternalID = firstValue(v.CustomerExternalID)
		t.Memo = v.Memo
		t.SubsidiaryExternalID = v.SubsidiaryCustRecord155
		tranDate, err := time.Parse("01/02/2006", v.TransactionDate)
		if err != nil {
			log.Print("failed to parse transaction date: ", err)
		}
		t.TranDate = tranDate
		t.TranID = v.TranID
		t.Amount = amount
		t.ParCSReference = v.CustbodyParcsRefBody // This is always blank. Is it the right field?
		t.CustomerCategory = firstValue(v.CustomerCustentitySILCustCategory)
		t.ParCSTranCode = firstText(v.CustcolParcsTranTypeCode)
		t.TranType = tranType

		rows[i] = t
	}
	return rows
}

func firstValue(v []SelectValue) string {
	if len(v) == 0 {
		return ""
	}
	return v[0].Value
}

func firstText(v []SelectValue) string {
	if len(v) == 0 {
		return ""
	}
	return v[0].Text
}

func parseAmount(s string) int {
	if s == "" {
		return 0
	}

	f, err := strconv.ParseFloat(s, 32)
	if err != nil {
		log.Printf("failed to parse float from string: %s", s)
		return 0
	}
	return int(math.Round(f * 100))
}

func GroupTransactions(transactions []Transaction) ([]SubsidiaryTransactions, error) {
	groupedTransactions := map[string][]Transaction{}
	totals := map[string]int{}
	for _, t := range transactions {
		totals[t.SubsidiaryExternalID] = totals[t.SubsidiaryExternalID] + t.Amount
		groupedTransactions[t.SubsidiaryExternalID] = append(groupedTransactions[t.SubsidiaryExternalID], t)
	}

	totalTransactions := 0
	t := make([]SubsidiaryTransactions, 0, len(groupedTransactions))
	for subsidiary := range groupedTransactions {
		t = append(t, SubsidiaryTransactions{
			Subsidiary:   subsidiary,
			TotalAmount:  totals[subsidiary],
			Transactions: groupedTransactions[subsidiary],
		})
		totalTransactions += len(groupedTransactions[subsidiary])
	}
	if len(transactions) != totalTransactions {
		return nil, fmt.Errorf("total number of transactions in groups is not correct, expected %d, got %d",
			len(transactions), totalTransactions)
	}
	return t, nil
}

func MarkTransactionsSent(ctx context.Context, transactions []Transaction, cfg Config) error {
	sem := make(chan struct{}, min(len(transactions), MaxConcurrent))
	errCh := make(chan error, len(transactions))
	var wg sync.WaitGroup

	tMap := make(map[string]*Transaction, len(transactions))
	for i, t := range transactions {
		if _, ok := tMap[t.NetSuiteID]; ok {
			log.Printf("skipping duplicate transaction: %s", t.NetSuiteID)
			continue
		}
		tMap[t.NetSuiteID] = &transactions[i]
	}

	for _, t := range tMap {
		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
		}

		sem <- struct{}{}
		wg.Add(1)

		go func(t *Transaction) {
			defer wg.Done()
			defer func() { <-sem }()

			if err := MarkTransactionSent(t, cfg); err != nil {
				errCh <- err
				return
			}

			log.Printf("updated NetSuite transaction %v", t.NetSuiteID)
		}(t)
	}

	wg.Wait()
	close(errCh)

	if len(errCh) > 0 {
		errs := make([]error, 0, len(errCh))
		for err := range errCh {
			errs = append(errs, err)
		}
		return errors.Join(errs...)
	}

	return nil
}

func MarkTransactionSent(t *Transaction, cfg Config) error {
	url, err := transactionURL(t.NetSuiteID, t.TranType, cfg.NetSuiteRealm)
	if err != nil {
		return err
	}

	requestBody := fmt.Sprintf(`{"custbody_sent_to_parcs":true,"custbody_date_sent_to_parcs":"%s"}`,
		time.Now().UTC().Format(time.RFC3339))

	if err := httpRequest(cfg.client, http.MethodPatch, url, requestBody); err != nil {
		return fmt.Errorf("failed to update %v transaction %v: %w", t.TranType, t.NetSuiteID, err)
	}

	log.Printf("updated NetSuite transaction %v", t.NetSuiteID)
	return nil
}

func transactionURL(transactionID, transactionType, realm string) (string, error) {
	transactionTypes := map[string]string{
		CashRefund:      "cashRefund",
		CashSale:        "cashSale",
		CustomerDeposit: "customerDeposit",
		CustomerRefund:  "customerRefund",
	}
	endpoint, ok := transactionTypes[transactionType]
	if !ok {
		return "", fmt.Errorf("invalid transaction type: %v", transactionType)
	}

	host := strings.Replace(realm, "_", "-", 1)
	url := fmt.Sprintf("https://%s.suitetalk.api.netsuite.com/services/rest/record/v1/%s/%s",
		host, endpoint, transactionID)
	return url, nil
}

func httpRequest(client *http.Client, method, url, body string) error {
	if client == nil {
		return errors.New("HTTP client has not been initialized")
	}

	req, err := http.NewRequest(method, url, strings.NewReader(body))
	if err != nil {
		return fmt.Errorf("failed to create HTTP request: %w", err)
	}

	req.Header.Set("Content-Type", "application/json")

	resp, err := client.Do(req)
	if err != nil {
		return fmt.Errorf("failed to send transaction update: %w", err)
	}
	defer func() {
		err = resp.Body.Close()
		if err != nil {
			log.Println("failed to close response body")
		}
	}()

	if resp.StatusCode >= 300 {
		b, err := io.ReadAll(resp.Body)
		if err != nil {
			return fmt.Errorf("body read failed: %w", err)
		}
		return fmt.Errorf("call failed with status code %d, body: %s", resp.StatusCode, b)
	}

	return nil
}

func SendToWorkday(cfg Config, data []ssh.Document) error {
	sshClient, err := ssh.Connect(cfg.SFTPUsername, cfg.SFTPHost, cfg.SFTPPrivateKey)
	if err != nil {
		return fmt.Errorf("failed to connect SSH: %w", err)
	}
	defer func() {
		err := sshClient.Close()
		if err != nil {
			log.Fatalf("SSH client failure: %s", err)
		}
	}()

	sftpClient, err := sftp.NewClient(sshClient)
	if err != nil {
		return fmt.Errorf("failed to create sftp client: %w", err)
	}
	defer func() {
		err := sftpClient.Close()
		if err != nil {
			log.Fatalf("SFTP client failure: %s", err)
		}
	}()

	if err = ssh.UploadDocuments(sftpClient, data, cfg.SFTPDirectory); err != nil {
		return fmt.Errorf("failed to write data: %w", err)
	}

	return nil
}

// CreateXMLDocuments converts a list of SubsidiaryTransactions to a map of Documents keyed by subsidiary
func CreateXMLDocuments(st []SubsidiaryTransactions) (map[string]ssh.Document, error) {
	today := time.Now().Format(time.RFC3339)

	docs := make(map[string]ssh.Document)
	for _, t := range st {
		b, err := createXMLDocument(t)
		if err != nil {
			return nil, fmt.Errorf("XML error on %s: %w", t.Subsidiary, err)
		}

		doc := ssh.Document{
			Name:    fmt.Sprintf("%s_%s.xml", t.Subsidiary, today),
			Content: string(b),
		}

		if _, ok := docs[t.Subsidiary]; ok {
			return nil, fmt.Errorf("duplicate XML document: %s", t.Subsidiary)
		}

		docs[t.Subsidiary] = doc
	}
	return docs, nil
}

// createXMLDocument converts a SubsidiaryTransactions to an XMLDocument
func createXMLDocument(t SubsidiaryTransactions) ([]byte, error) {
	var w bytes.Buffer
	err := writeXML(t, &w)
	if err != nil {
		return nil, fmt.Errorf("failed to create XML: %w", err)
	}
	return w.Bytes(), nil
}

// writeXML creates XML data from a SubsidiaryTransactions batch.
func writeXML(st SubsidiaryTransactions, w io.Writer) error {
	batch := PMISBatch{
		Header: PMISHeader{
			BatchCount:    len(st.Transactions),
			BatchTotal:    float32(st.TotalAmount) / 100,
			OriginatingPP: st.Subsidiary,
		},
		Trans: make([]PMISTran, len(st.Transactions)),
	}

	for i, t := range st.Transactions {
		batch.Trans[i] = convertTransaction(t)
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

// convertTransaction makes a PMISTran from a Transaction for the XML generation process.
func convertTransaction(t Transaction) PMISTran {
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
