import sys
import time
import random

def print_colored_hello():
    colors = [
        '\033[91m',  # Red
        '\033[92m',  # Green
        '\033[93m',  # Yellow
        '\033[94m',  # Blue
        '\033[95m',  # Magenta
        '\033[96m',  # Cyan
        '\033[97m',  # White
    ]
    reset = '\033[0m'
    try:
        while True:
            color = random.choice(colors)
            sys.stdout.write(f'\r{color}Hello World!{reset}')
            sys.stdout.flush()
            time.sleep(0.5)
    except KeyboardInterrupt:
        print(f'\n{reset}程序结束')

if __name__ == '__main__':
    print_colored_hello()