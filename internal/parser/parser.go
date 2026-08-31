package parser

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/PuerkitoBio/goquery"
	"github.com/chromedp/chromedp"
)

// Структура хранящая данные о товаре для записи в в БД
type ProductInfo struct {
	// Name      string // Как будто не обязательно, поскольку можно взять у юзера
	Store     string
	Price     int
	CheckedAt time.Time
}

type Parser interface {
	Parse(ctx context.Context, url string) (ProductInfo, error)
}

// пока перенес в registry
// type Parser interface {
// 	Parse(ctx context.Context, url string) (ProductInfo, error)
// }

func NormalizeToKopecks(input string) (int, error) {
	// 1. Предварительная очистка
	input = strings.ToLower(input)
	input = strings.TrimSpace(input)

	// 2. Проверяем явный формат с текстом "руб" и "коп" (например, "14 руб 50 коп" или "1.400 руб")
	// Сначала убираем точки-разделители тысяч, если перед ними и после них цифры
	rxThousandDot := regexp.MustCompile(`(\d+)\.(\d{3})`)

	// Если в строке есть "коп" или "руб", точки внутри цифр — это почти наверняка тысячи
	if strings.Contains(input, "коп") || strings.Contains(input, "к") || strings.Contains(input, "руб") || strings.Contains(input, "р") {
		// Заменяем "1.400" на "1400"
		for rxThousandDot.MatchString(input) {
			input = rxThousandDot.ReplaceAllString(input, "$1$2")
		}
	}

	// Обрабатываем формат "X руб Y коп" после очистки тысяч
	rxRubKop := regexp.MustCompile(`(?:(\d+)\s*(?:руб|р))?\s*(?:(\d+)\s*(?:коп|к))`)
	if rxRubKop.MatchString(input) {
		matches := rxRubKop.FindStringSubmatch(input)
		var rubles, kopecks int
		if matches[1] != "" {
			rubles, _ = strconv.Atoi(matches[1])
		}
		if matches[2] != "" {
			kopecks, _ = strconv.Atoi(matches[2])
		}
		return rubles*100 + kopecks, nil
	}

	// 3. Если это чисто числовой формат (например, "14.000,50" или "1.400.000")
	// Если есть и точка, и запятая, то запятая — это копейки, а точки — тысячи.
	if strings.Contains(input, ".") && strings.Contains(input, ",") {
		input = strings.ReplaceAll(input, ".", "")  // Удаляем тысячи
		input = strings.ReplaceAll(input, ",", ".") // Заменяем копейки на точку
	} else if strings.Contains(input, ".") {
		// Если есть ТОЛЬКО точки (например, "14.000" или "14.50")
		// Считаем количество точек
		dotCount := strings.Count(input, ".")
		if dotCount > 1 {
			// Больше одной точки (1.400.000) — это точно тысячи, удаляем их все
			input = strings.ReplaceAll(input, ".", "")
		} else {
			// Если точка одна (14.000 или 14.50), смотрим сколько цифр после нее
			parts := strings.Split(input, ".")
			if len(parts[1]) == 3 {
				// Если после точки ровно 3 цифры ("14.000"), то это тысячи!
				input = parts[0] + parts[1]
			}
			// Если после точки 1, 2 или больше 3 цифр (14.50, 14.5), то это копейки. Оставляем точку.
		}
	} else if strings.Contains(input, ",") {
		// Если только запятая (14,50), то это копейки
		input = strings.ReplaceAll(input, ",", ".")
	}

	// 4. Финальный парсинг стандартного Float числа (которое мы получили после всех замен)
	rxFloat := regexp.MustCompile(`(\d+)(?:\.(\d+))?`)
	if rxFloat.MatchString(input) {
		matches := rxFloat.FindStringSubmatch(input)
		rublesStr := matches[1]
		kopecksStr := matches[2]

		rubles, _ := strconv.Atoi(rublesStr)
		var kopecks int

		if kopecksStr != "" {
			switch len(kopecksStr) {
			case 1:
				kopecksStr += "0"
			case 2:
				break
			default:
				kopecksStr = kopecksStr[:2]
			}
			kopecks, _ = strconv.Atoi(kopecksStr)
		}

		return rubles*100 + kopecks, nil
	}

	return 0, fmt.Errorf("не удалось распознать формат цены: %s", input)
}

type BaltOptTorg struct {
}

func (b *BaltOptTorg) Parse(ctx context.Context, url string) (ProductInfo, error) {

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return ProductInfo{}, err
	}

	// 3. Отправляем запрос через стандартный HTTP-клиент
	res, err := http.DefaultClient.Do(req)
	if err != nil {
		return ProductInfo{}, err
	}
	defer res.Body.Close()

	if res.StatusCode != 200 {
		return ProductInfo{}, err
	}

	doc, err := goquery.NewDocumentFromReader(res.Body)
	if err != nil {
		return ProductInfo{}, err
	}

	price := doc.Find(".product__price").Text()

	// Нужно нормализовать
	result, err := NormalizeToKopecks(price)
	if err != nil {
		return ProductInfo{}, err
	}

	return ProductInfo{
		// Name:      doc.Find(".product__title").Text(),
		Store:     "БалтОптТорг",
		Price:     result,
		CheckedAt: time.Now(),
	}, nil
}

//////////////////////////////////////////////////////////////////////////////////
// Перестал работать

type OzonParser struct {
}

func (b *OzonParser) Parse(ctx context.Context, url string) (ProductInfo, error) {
	opts := append(chromedp.DefaultExecAllocatorOptions[:],
		chromedp.Flag("headless", false),
		chromedp.Flag("no-sandbox", true),
		chromedp.WindowSize(1920, 1080),
	)

	allocCtx, allocCancel := chromedp.NewExecAllocator(ctx, opts...)
	defer allocCancel()

	browserCtx, browserCancel := chromedp.NewContext(allocCtx)
	defer browserCancel()

	timeoutCtx, timeoutCancel := context.WithTimeout(browserCtx, 40*time.Second)
	defer timeoutCancel()

	var htmlContent string

	log.Println("Запуск Chrome и переход на Ozon...")

	err := chromedp.Run(
		timeoutCtx,

		chromedp.Navigate(url),

		// Даем 5 секунд на полную загрузку всех скриптов
		chromedp.Sleep(5*time.Second),

		// Забираем ВЕСЬ исходный HTML-код страницы
		chromedp.OuterHTML(`html`, &htmlContent, chromedp.ByQuery),
	)

	if err != nil {
		return ProductInfo{}, fmt.Errorf("Ошибка получения HTML страницы: %w", err)
	}

	// Ищем в HTML текст вида "price":"41500" или "price":41500
	// Ozon всегда хранит актуальную цену в JSON-состоянии страницы (внутри тегов <script>)
	re := regexp.MustCompile(`"price"\s*:\s*"?(\d+)"?`)
	matches := re.FindAllStringSubmatch(htmlContent, -1)

	if len(matches) == 0 {
		return ProductInfo{}, fmt.Errorf("Не удалось найти блок цены в JSON коде страницы")
	}

	// Обычно первая или вторая найденная цена в коде — это и есть цена товара.
	// Пройдемся по найденным значениям и выберем адекватную цену (не 0, не 1 и не id товара)
	var finalPrice int

	if finalPrice == 0 {
		// Если адекватная цена не отфильтровалась, берем просто самое первое найденное число
		finalPrice, _ = strconv.Atoi(matches[0][1])
	}

	// Переводим в копейки (в JSON Ozon цена уже идет в целых рублях)
	kopecks := finalPrice * 100

	return ProductInfo{
		// Name:      , // хз откуда это взять
		Store:     "Ozon",
		Price:     kopecks,
		CheckedAt: time.Now(),
	}, nil

}

//////////////////////////////////////////////////////////////////////////////////

type RegardParser struct{}

type RegardProductJSON struct {
	Type   string `json:"@type"`
	Offers struct {
		Price         json.RawMessage `json:"price"`
		PriceCurrency string          `json:"priceCurrency"`
	} `json:"offers"`
}

func (r *RegardParser) Parse(
	ctx context.Context,
	productURL string,
) (ProductInfo, error) {

	req, err := http.NewRequestWithContext(
		ctx,
		http.MethodGet,
		productURL,
		nil,
	)
	if err != nil {
		return ProductInfo{}, err
	}

	req.Header.Set("User-Agent", "PriceMon/0.1")

	res, err := http.DefaultClient.Do(req)
	if err != nil {
		return ProductInfo{}, err
	}
	defer res.Body.Close()

	if res.StatusCode != http.StatusOK {
		return ProductInfo{},
			fmt.Errorf("unexpected status code: %d", res.StatusCode)
	}

	doc, err := goquery.NewDocumentFromReader(res.Body)
	if err != nil {
		return ProductInfo{}, err
	}

	var product RegardProductJSON
	found := false

	doc.Find(`script[type="application/ld+json"]`).
		EachWithBreak(func(_ int, s *goquery.Selection) bool {

			var candidate RegardProductJSON

			if err := json.Unmarshal(
				[]byte(s.Text()),
				&candidate,
			); err != nil {
				return true
			}

			if candidate.Type == "Product" {
				product = candidate
				found = true
				return false
			}

			return true
		})

	if !found {
		return ProductInfo{},
			fmt.Errorf("product JSON-LD not found")
	}

	rawPrice := strings.Trim(
		string(product.Offers.Price),
		`"`,
	)

	price, err := NormalizeToKopecks(rawPrice)
	if err != nil {
		return ProductInfo{}, fmt.Errorf(
			"normalize price %q: %w",
			rawPrice,
			err,
		)
	}

	return ProductInfo{
		Store:     "Regard",
		Price:     price,
		CheckedAt: time.Now(),
	}, nil
}

//////////////////////////////////////////////////////////////////////////////////
