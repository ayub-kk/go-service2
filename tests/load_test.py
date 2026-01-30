import json
import random
import time
import threading
import statistics
from datetime import datetime
from urllib.request import Request, urlopen
from urllib.error import URLError, HTTPError
from concurrent.futures import ThreadPoolExecutor, as_completed

class LoadTester:
    def __init__(self, base_url, num_workers=10, duration=300):
        self.base_url = base_url
        self.num_workers = num_workers
        self.duration = duration
        self.results = []
        self.lock = threading.Lock()
        self.stop_event = threading.Event()
        self.total_requests = 0
        self.successful_requests = 0
        
    def generate_metric(self):
        """Генерация тестовых метрик"""
        return {
            "device_id": f"device_{random.randint(1, 1000)}",
            "cpu_usage": random.uniform(0.1, 0.9),
            "memory_usage": random.uniform(0.1, 0.8),
            "rps": random.uniform(100, 1000),
            "latency_ms": random.uniform(1, 100)
        }
    
    def send_request(self, url, data):
        """Отправка HTTP-запроса без внешних зависимостей"""
        start_time = time.time()
        
        try:
            # Подготовка запроса
            headers = {'Content-Type': 'application/json'}
            json_data = json.dumps(data).encode('utf-8')
            
            req = Request(url, data=json_data, headers=headers, method='POST')
            
            # Отправка запроса
            with urlopen(req, timeout=5) as response:
                response_code = response.getcode()
                response_body = response.read()
                
                latency = (time.time() - start_time) * 1000  # в мс
                
                return {
                    'status': response_code,
                    'latency': latency,
                    'success': response_code == 202
                }
                
        except HTTPError as e:
            return {
                'status': e.code,
                'latency': (time.time() - start_time) * 1000,
                'success': False,
                'error': str(e)
            }
        except URLError as e:
            return {
                'status': 0,
                'latency': (time.time() - start_time) * 1000,
                'success': False,
                'error': str(e)
            }
        except Exception as e:
            return {
                'status': 0,
                'latency': (time.time() - start_time) * 1000,
                'success': False,
                'error': str(e)
            }
    
    def worker(self, worker_id):
        """Рабочий поток для отправки запросов"""
        url = f"{self.base_url}/metrics/ingest"
        
        while not self.stop_event.is_set():
            try:
                metric = self.generate_metric()
                result = self.send_request(url, metric)
                
                with self.lock:
                    self.total_requests += 1
                    if result['success']:
                        self.successful_requests += 1
                        self.results.append({
                            'timestamp': datetime.now(),
                            'status': result['status'],
                            'latency': result['latency'],
                            'worker': worker_id
                        })
                    else:
                        print(f"Worker {worker_id} error: {result.get('error', 'Unknown error')}")
                
                # Случайная задержка между запросами
                time.sleep(random.uniform(0.001, 0.01))
                
            except Exception as e:
                print(f"Worker {worker_id} error: {e}")
    
    def run_concurrent(self):
        """Запуск нагрузочного теста с использованием ThreadPoolExecutor"""
        print(f"Starting load test with {self.num_workers} workers for {self.duration} seconds")
        
        # Запуск рабочих потоков
        threads = []
        for i in range(self.num_workers):
            thread = threading.Thread(target=self.worker, args=(i,))
            thread.daemon = True
            thread.start()
            threads.append(thread)
        
        # Ожидание завершения
        time.sleep(self.duration)
        self.stop_event.set()
        
        # Даем время завершиться
        time.sleep(2)
        
        # Анализ результатов
        self.analyze_results()
    
    def analyze_results(self):
        """Анализ результатов тестирования"""
        if not self.results:
            print("No results collected")
            return
        
        total = self.total_requests
        successful = self.successful_requests
        failed = total - successful
        
        latencies = [r['latency'] for r in self.results]
        
        rps = total / self.duration
        
        print("\n" + "="*50)
        print("РЕЗУЛЬТАТЫ НАГРУЗОЧНОГО ТЕСТИРОВАНИЯ")
        print("="*50)
        print(f"Общее время теста: {self.duration} секунд")
        print(f"Всего запросов: {total}")
        print(f"Успешных запросов: {successful}")
        print(f"Неудачных запросов: {failed}")
        print(f"RPS: {rps:.2f}")
        print(f"Успешность: {(successful/total*100):.2f}%")
        
        if latencies:
            latencies_sorted = sorted(latencies)
            n = len(latencies_sorted)
            
            print(f"\nЗадержка (мс):")
            print(f"  Средняя: {statistics.mean(latencies):.2f}")
            print(f"  Медиана: {statistics.median(latencies):.2f}")
            if n > 0:
                print(f"  P95: {latencies_sorted[int(n * 0.95)]:.2f}")
                print(f"  P99: {latencies_sorted[int(n * 0.99)]:.2f}")
                print(f"  Максимальная: {max(latencies):.2f}")
                print(f"  Минимальная: {min(latencies):.2f}")
        
        # Проверка аналитики
        self.check_analytics()
    
    def check_analytics(self):
        """Проверка работы аналитики"""
        try:
            # Получение текущей аналитики
            analytics_url = f"{self.base_url}/analytics/current"
            req = Request(analytics_url, method='GET')
            
            with urlopen(req, timeout=5) as response:
                if response.getcode() == 200:
                    data = response.read().decode('utf-8')
                    analytics = json.loads(data)
                    
                    print(f"\n" + "="*50)
                    print("ДАННЫЕ АНАЛИТИКИ СЕРВИСА")
                    print("="*50)
                    print(f"Текущий RPS: {analytics.get('current_rps', 'N/A'):.2f}")
                    print(f"Скользящее среднее: {analytics.get('rolling_average', 'N/A'):.2f}")
                    print(f"Процент аномалий: {analytics.get('anomaly_rate', 0)*100:.2f}%")
                    print(f"Всего метрик: {analytics.get('total_metrics', 'N/A')}")
                    print(f"Всего аномалий: {analytics.get('total_anomalies', 'N/A')}")
                    
                    if analytics.get('last_anomaly_time'):
                        print(f"Последняя аномалия: {analytics.get('last_anomaly_time')}")
        
        except Exception as e:
            print(f"Ошибка при проверке аналитики: {e}")

def simple_load_test(base_url, requests_per_second=100, duration=30):
    """Упрощенный нагрузочный тест для быстрой проверки"""
    print(f"\nПростой нагрузочный тест: {requests_per_second} RPS в течение {duration} секунд")
    
    interval = 1.0 / requests_per_second
    results = []
    
    def make_request():
        try:
            data = {
                "device_id": f"device_{random.randint(1, 100)}",
                "cpu_usage": random.random(),
                "rps": random.uniform(50, 150)
            }
            
            start = time.time()
            req = Request(
                f"{base_url}/metrics/ingest",
                data=json.dumps(data).encode('utf-8'),
                headers={'Content-Type': 'application/json'},
                method='POST'
            )
            
            with urlopen(req, timeout=2) as response:
                latency = (time.time() - start) * 1000
                return {'success': True, 'latency': latency}
                
        except Exception as e:
            return {'success': False, 'error': str(e)}
    
    start_time = time.time()
    request_count = 0
    
    while time.time() - start_time < duration:
        request_start = time.time()
        
        result = make_request()
        results.append(result)
        request_count += 1
        
        if result['success']:
            print(f"✓ Запрос {request_count}: {result['latency']:.1f} мс")
        else:
            print(f"✗ Запрос {request_count}: {result.get('error', 'Error')}")
        
        # Поддержание заданного RPS
        elapsed = time.time() - request_start
        sleep_time = max(0, interval - elapsed)
        time.sleep(sleep_time)
    
    # Статистика
    successful = sum(1 for r in results if r['success'])
    latencies = [r['latency'] for r in results if r['success']]
    
    print(f"\nРезультаты:")
    print(f"Всего запросов: {request_count}")
    print(f"Успешно: {successful} ({successful/request_count*100:.1f}%)")
    print(f"RPS: {request_count/duration:.1f}")
    
    if latencies:
        print(f"Средняя задержка: {statistics.mean(latencies):.1f} мс")
        print(f"Максимальная задержка: {max(latencies):.1f} мс")

if __name__ == "__main__":
    import sys
    
    # URL сервиса
    if len(sys.argv) > 1:
        base_url = sys.argv[1]
    else:
        base_url = "http://localhost:8080"
    
    print(f"Тестируем сервис по адресу: {base_url}")
    
    # Проверка доступности сервиса
    try:
        req = Request(f"{base_url}/health", method='GET')
        with urlopen(req, timeout=5) as response:
            if response.getcode() == 200:
                print("Сервис доступен!")
            else:
                print(f"Сервис вернул код: {response.getcode()}")
                sys.exit(1)
    except Exception as e:
        print(f"Сервис недоступен: {e}")
        print("\nУбедитесь, что сервис запущен:")
        print("1. kubectl port-forward service/go-service 8080:80 -n go-service")
        print("2. Или используйте minikube service go-service -n go-service --url")
        sys.exit(1)
    
    # Выбор режима тестирования
    print("\nВыберите режим тестирования:")
    print("1. Быстрая проверка (30 секунд)")
    print("2. Полный нагрузочный тест (5 минут)")
    
    choice = input("Введите 1 или 2: ").strip()
    
    if choice == "1":
        simple_load_test(base_url, requests_per_second=50, duration=30)
    else:
        # Полный нагрузочный тест
        tester = LoadTester(
            base_url=base_url,
            num_workers=50,  # 50 воркеров для ~1000 RPS
            duration=300     # 5 минут
        )
        
        tester.run_concurrent()