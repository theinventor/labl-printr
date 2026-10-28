package labelpolicy

import "testing"

func TestShopifyPOSQRPolicy(t *testing.T) {
	for _, u := range []string{"https://andherson.com/products/landscape-journal-medium?variant=67909533925455", "https://shop.myshopify.com/products/wallet"} {
		if err := Validate("inventory-shopify-pos", map[string]string{"url": u}); err != nil {
			t.Errorf("valid: %s: %v", u, err)
		}
	}
	for _, u := range []string{"", "http://andherson-vm.tailb8614f.ts.net:3000/products/id", "https://andherson-vm.tailb8614f.ts.net/products/id", "https://10.1.0.75/products/id", "https://localhost/products/id", "https://studio/products/id", "https://studio.local/products/id", "https://admin.shopify.com/store/products/1", "https://andherson.com/admin/products/1", "http://andherson.com/products/id", "https://user:pass@andherson.com/products/id", "https://andherson.com:3000/products/id", "https://andherson.com/products/id?variant=undefined", "https://andherson.com/products/id?variant=1&variant=2"} {
		if Validate("inventory-shopify-pos", map[string]string{"url": u}) == nil {
			t.Errorf("accepted unsafe POS QR: %s", u)
		}
	}
	if err := Validate("inventory", map[string]string{"url": "https://inv.local/items/1"}); err != nil {
		t.Fatal("generic inventory changed", err)
	}
}
