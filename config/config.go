package config

import (
	"context"
	"fmt"
	"log"
	"reflect"
	"strings"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/service/ssm"
	"github.com/aws/aws-sdk-go-v2/service/ssm/types"
)

// ReadParameterStore reads parameters from AWS SSM Parameter Store and saves the values in struct pointed to by cfg.
func ReadParameterStore(path string, cfg any) {
	log.Printf("reading parameters from SSM path: %s", path)

	c, err := config.LoadDefaultConfig(context.Background())
	if err != nil {
		panic("failed to get AWS config for parameter store: " + err.Error())
	}
	client := ssm.NewFromConfig(c)

	params, err := getAllParameters(client, path)
	if err != nil {
		panic("failed to get parameters from SSM: " + err.Error())
	}

	for _, v := range params {
		if v.Name == nil {
			log.Println("SSM returned a parameter with nil name")
			continue
		}
		name := strings.TrimPrefix(*v.Name, path)
		name = strings.TrimPrefix(name, "/")

		fields := map[string]any{"name": name}
		if v.Value == nil {
			log.Printf("SSM returned parameter with nil value, %v", fields)
			continue
		}

		err = SetStructField(&cfg, name, *v.Value)
		if err != nil {
			log.Printf("readParameterStore: %s", err)
			continue
		}
		log.Printf("parameter %q read from SSM Parameter Store", name)
	}
}

// getAllParameters retrieves all parameters from the given path on Parameter Store
func getAllParameters(client *ssm.Client, path string) ([]types.Parameter, error) {
	var parameters []types.Parameter
	var token *string
	for {
		out, err := client.GetParametersByPath(context.Background(), &ssm.GetParametersByPathInput{
			Path:           &path,
			WithDecryption: aws.Bool(true),
			NextToken:      token,
		})
		if err != nil {
			return nil, fmt.Errorf("failed to get parameters from SSM: %w", err)
		}

		parameters = append(parameters, out.Parameters...)
		if out.NextToken == nil || len(out.Parameters) == 0 {
			break
		}
		token = out.NextToken
	}
	return parameters, nil
}

// SetStructField uses reflection to set the value of a struct field by name
func SetStructField(structPtr any, fieldName string, value any) error {
	ptrValue := reflect.ValueOf(structPtr)
	if ptrValue.Kind() != reflect.Ptr || ptrValue.Elem().Kind() != reflect.Struct {
		return fmt.Errorf("structPtr must be a pointer to a struct")
	}

	structValue := ptrValue.Elem()
	fieldValue := structValue.FieldByName(fieldName)

	if !fieldValue.IsValid() {
		return fmt.Errorf("field %q does not exist in the struct", fieldName)
	}

	if !fieldValue.CanSet() {
		return fmt.Errorf("field cannot be set (possibly unexported)")
	}

	val := reflect.ValueOf(value)
	if fieldValue.Type() != val.Type() {
		return fmt.Errorf("field expects a value of type %s, got %s", fieldValue.Type(), val.Type())
	}

	fieldValue.Set(val)
	return nil
}
