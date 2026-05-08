package controller

import (
	"errors"
	"net/http"
	"strconv"
	"strings"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
	"github.com/gin-gonic/gin"
)

func MangouAdminListPaymentProducts(c *gin.Context) {
	provider := strings.TrimSpace(strings.ToLower(c.Query("provider")))
	query := model.DB.Model(&model.PaymentProduct{})
	if provider != "" {
		query = query.Where("provider = ?", provider)
	}
	var products []model.PaymentProduct
	if err := query.Order("sort_order asc, id asc").Find(&products).Error; err != nil {
		common.ApiError(c, err)
		return
	}
	common.ApiSuccess(c, products)
}

func MangouAdminCreatePaymentProduct(c *gin.Context) {
	var product model.PaymentProduct
	if err := c.ShouldBindJSON(&product); err != nil {
		common.ApiError(c, err)
		return
	}
	if err := validateMangouPaymentProduct(&product); err != nil {
		common.ApiError(c, err)
		return
	}
	if err := model.DB.Create(&product).Error; err != nil {
		common.ApiError(c, err)
		return
	}
	common.ApiSuccess(c, product)
}

func MangouAdminUpdatePaymentProduct(c *gin.Context) {
	id, err := strconv.Atoi(c.Param("id"))
	if err != nil || id <= 0 {
		c.JSON(http.StatusOK, gin.H{"success": false, "message": "invalid payment product id"})
		return
	}
	var req model.PaymentProduct
	if err := c.ShouldBindJSON(&req); err != nil {
		common.ApiError(c, err)
		return
	}
	var product model.PaymentProduct
	if err := model.DB.First(&product, id).Error; err != nil {
		common.ApiError(c, err)
		return
	}
	product.Provider = req.Provider
	product.ProductId = req.ProductId
	product.Name = req.Name
	product.Currency = req.Currency
	product.Price = req.Price
	product.Quota = req.Quota
	product.Tier = req.Tier
	product.Status = req.Status
	product.SortOrder = req.SortOrder
	product.Metadata = req.Metadata
	if err := validateMangouPaymentProduct(&product); err != nil {
		common.ApiError(c, err)
		return
	}
	if err := model.DB.Save(&product).Error; err != nil {
		common.ApiError(c, err)
		return
	}
	common.ApiSuccess(c, product)
}

func validateMangouPaymentProduct(product *model.PaymentProduct) error {
	product.Provider = strings.TrimSpace(strings.ToLower(product.Provider))
	product.ProductId = strings.TrimSpace(product.ProductId)
	product.Tier = strings.TrimSpace(product.Tier)
	if product.Provider == "" {
		return errors.New("provider is required")
	}
	if product.ProductId == "" {
		return errors.New("product_id is required")
	}
	if product.Quota <= 0 {
		return errors.New("quota must be positive")
	}
	if product.Price < 0 {
		return errors.New("price cannot be negative")
	}
	if product.Status == 0 {
		product.Status = model.PaymentProductStatusEnabled
	}
	return nil
}
