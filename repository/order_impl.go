package repository

import (
	"github.com/jinzhu/gorm"
	"github.com/shopicano/shopicano-backend/app"
	"github.com/shopicano/shopicano-backend/errors"
	"github.com/shopicano/shopicano-backend/log"
	"github.com/shopicano/shopicano-backend/models"
	payment_gateways "github.com/shopicano/shopicano-backend/payment-gateways"
	"github.com/shopicano/shopicano-backend/utils"
	"github.com/shopicano/shopicano-backend/validators"
	"net/http"
	"time"
)

type OrderRepositoryImpl struct {
}

var orderRepository OrderRepository

func NewOrderRepository() OrderRepository {
	if orderRepository == nil {
		orderRepository = &OrderRepositoryImpl{}
	}
	return orderRepository
}

func (os *OrderRepositoryImpl) CreateOrder(v *validators.ReqOrderCreate) (*models.OrderDetails, error) {
	tx := app.DB().Begin()

	pe := errors.NewPreparedError()

	order := models.Order{
		ID:         utils.NewUUID(),
		Hash:       utils.NewShortUUID(),
		Status:     models.Pending,
		GrandTotal: 0,
		SubTotal:   0,
		CreatedAt:  time.Now().UTC(),
		IsPaid:     false,
		UserID:     v.UserID,
	}

	var orderedProducts []models.OrderedProduct

	s := models.Store{}
	if err := tx.Table(s.TableName()).First(&s, "id = ?", v.StoreID).Error; err != nil {
		tx.Rollback()

		if err != gorm.ErrRecordNotFound {
			pe.Err.Add("shop", "not found")
			pe.Err.Add("shop", s.ID)
			pe.Status = http.StatusNotFound
			return nil, pe
		}
		return nil, err
	}

	order.StoreID = s.ID

	productUseCase := NewProductRepository()

	hasShippableProduct := false
	for _, p := range v.Products {
		prodDetails, err := productUseCase.GetProductWithStore(v.StoreID, p.ID)
		if err != nil {
			tx.Rollback()

			if err == gorm.ErrRecordNotFound {
				pe.Err.Add("product", "not found")
				pe.Err.Add("product", p.ID)
				pe.Status = http.StatusNotFound
				return nil, pe
			}
			return nil, err
		}

		log.Log().Infoln("Got product details")

		prod := models.Product{}
		if affectedRows := tx.Table(prod.TableName()).
			Where("id = ? AND store_id = ? AND stock >= ?", p.ID, s.ID, p.Quantity).
			UpdateColumn("stock", gorm.Expr("stock - ?", p.Quantity)).
			RowsAffected; affectedRows == 0 {
			tx.Rollback()

			pe.Err.Add("product", "out of stock")
			pe.Err.Add("product", p.ID)
			pe.Status = http.StatusBadRequest
			return nil, pe
		}

		if prodDetails.IsShippable {
			hasShippableProduct = true
		}

		oc := models.OrderedProduct{
			OrderID:   order.ID,
			Quantity:  p.Quantity,
			Price:     prodDetails.Price,
			ProductID: prodDetails.ID,
			SubTotal:  p.Quantity * prodDetails.Price,
			Name:      prodDetails.Name,
		}

		totalTax := 0
		totalVat := 0
		for _, ac := range prodDetails.AdditionalCharges {
			if ac.ChargeType == models.Vat {
				totalVat += ac.CalculateAdditionalCharge(oc.SubTotal)
			} else if ac.ChargeType == models.Tax {
				totalTax += ac.CalculateAdditionalCharge(oc.SubTotal)
			}
		}
		oc.TotalVat = totalVat
		oc.TotalTax = totalTax

		orderedProducts = append(orderedProducts, oc)

		order.SubTotal += oc.SubTotal
		order.TotalTax += totalTax
		order.TotalVat += totalVat
	}

	if hasShippableProduct && v.ShippingMethodID == nil {
		tx.Rollback()

		pe.Err.Add("shipping_method_id", "is required")
		pe.Status = http.StatusBadRequest
		return nil, pe
	}

	if hasShippableProduct && v.ShippingAddressID == nil {
		tx.Rollback()

		pe.Err.Add("shipping_address_id", "is required")
		pe.Status = http.StatusBadRequest
		return nil, pe
	}

	if hasShippableProduct {
		order.ShippingAddressID = v.ShippingAddressID

		shippingMethod := models.ShippingMethod{}
		if err := tx.Table(shippingMethod.TableName()).First(&shippingMethod, "id = ?", v.ShippingMethodID).Error; err != nil {
			tx.Rollback()

			if err == gorm.ErrRecordNotFound {
				pe.Err.Add("shipping_method", "not found")
				pe.Status = http.StatusNotFound
				return nil, pe
			}
			return nil, err
		}

		order.ShippingMethodID = v.ShippingMethodID
		order.ShippingCharge = shippingMethod.CalculateDeliveryCharge(0)
	}

	order.BillingAddressID = v.BillingAddressID
	billingAddress := models.Address{}
	if err := tx.Table(billingAddress.TableName()).Where("id = ?", order.BillingAddressID).First(&billingAddress).Error; err != nil {
		tx.Rollback()

		if err == gorm.ErrRecordNotFound {
			pe.Err.Add("billing_address", "not found")
			pe.Status = http.StatusNotFound
			return nil, pe
		}
		return nil, err
	}

	// Calculating grand total
	order.GrandTotal = order.SubTotal + order.TotalTax + order.TotalVat + order.ShippingCharge

	paymentMethod := models.PaymentMethod{}
	if err := tx.Table(paymentMethod.TableName()).First(&paymentMethod, "id = ?", v.PaymentMethodID).Error; err != nil {
		tx.Rollback()

		if err == gorm.ErrRecordNotFound {
			pe.Err.Add("payment_method", "not found")
			pe.Status = http.StatusNotFound
			return nil, pe
		}
		return nil, err
	}

	order.PaymentMethodID = v.PaymentMethodID
	order.PaymentProcessingFee = paymentMethod.CalculateProcessingFee(order.GrandTotal)

	if err := tx.Table(order.TableName()).Create(&order).Error; err != nil {
		tx.Rollback()
		return nil, err
	}

	for _, op := range orderedProducts {
		if err := tx.Table(op.TableName()).Create(&op).Error; err != nil {
			tx.Rollback()
			return nil, err
		}
	}

	orderDetails := &models.OrderDetails{
		ID:                   order.ID,
		TotalTax:             order.TotalTax,
		SubTotal:             order.SubTotal,
		GrandTotal:           order.GrandTotal,
		TotalVat:             order.TotalVat,
		StoreID:              order.StoreID,
		UpdatedAt:            order.UpdatedAt,
		CreatedAt:            order.CreatedAt,
		UserID:               order.UserID,
		ShippingCharge:       order.ShippingCharge,
		Hash:                 order.Hash,
		IsPaid:               order.IsPaid,
		Status:               order.Status,
		PaymentProcessingFee: order.PaymentProcessingFee,
		PaymentMethodID:      paymentMethod,
		BillingAddressID:     billingAddress,
		ShippingAddressID:    nil,
		ShippingMethodID:     nil,
		Products:             orderedProducts,
		CompletedAt:          order.CompletedAt,
		ConfirmedAt:          order.ConfirmedAt,
		PaidAt:               order.PaidAt,
		PaymentGateway:       payment_gateways.GetActivePaymentGateway().GetName(),
	}

	res, err := payment_gateways.GetActivePaymentGateway().Pay(orderDetails)
	if err != nil {
		tx.Rollback()
		return nil, err
	}

	order.PaymentGatewayReferenceID = res.ReferenceID
	orderDetails.PaymentGatewayReferenceID = res.ReferenceID

	if err := tx.Commit().Error; err != nil {
		return nil, err
	}
	return orderDetails, nil
}