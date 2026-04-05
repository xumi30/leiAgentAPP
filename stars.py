import tkinter as tk
from tkinter import messagebox
import random
import ti
class StarAnimation:
    def __init__(self):
        self.root = tk.Tk()
        self.canvas = tk.Canvas(self.root, width=800, height=600, bg="black")
        self.canvas.pack()
        self.stars = []
        self.create_stars()
    
    def create_stars(self):
        for _ in range(50):
            size = random.randint(30, 60)
            color = "#{:06x}".format(random.randint(0, 0xFFFFFF))
            x = random.randint(0, 800-size)
            y = random.randint(0, 600-size)
            star = self.canvas.create_rectangle(x, y, x+size, y+size, fill=color, outline="")
            self.stars.append([star, size, x, y])
    
    def move_stars(self):
        for idx, (star_id, size, x, y) in enumerate(self.stars):
            new_x = x + random.choice([-2, 2]) * 5
            new_y = y + random.choice([-2, 2]) * 5
            self.canvas.move(star_id, new_x, new_y)
            self.stars[idx] = [star_id, size, new_x, new_y]
        self.root.after(100, self.move_stars)
    
    def run(self):
        self.move_stars()
        self.root.mainloop()

if __name__ == "__main__":
    app = StarAnimation()
    app.run()